// Package transport sends signed requests to the gateway and classifies
// transport failures for retry and idempotency decisions.
package transport

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/http/httptrace"
	"strconv"
	"sync/atomic"
	"time"
)

// MaxResponseBytes caps how much of a response body is read, so a
// misbehaving gateway cannot exhaust memory or hold a connection open.
const MaxResponseBytes = 1 << 20 // 1 MiB

// Error wraps a failed attempt with whether any byte of the request reached
// the socket, as observed by httptrace rather than inferred from the error
// value.
type Error struct {
	Err            error
	RequestNotSent bool
}

func (e *Error) Error() string { return e.Err.Error() }
func (e *Error) Unwrap() error { return e.Err }

// Attempt performs one HTTP request and reports whether any request bytes
// reached the socket. WroteHeaderField and WroteRequest are the two
// httptrace hooks that fire once transmission has begun; a request that
// fails during DNS resolution, dial, or TLS handshake triggers neither.
func Attempt(ctx context.Context, client *http.Client, method, url string, body []byte, headers http.Header) (*http.Response, bool, error) {
	var wrote atomic.Bool
	trace := &httptrace.ClientTrace{
		WroteHeaderField: func(string, []string) { wrote.Store(true) },
		WroteRequest:     func(httptrace.WroteRequestInfo) { wrote.Store(true) },
	}
	ctx = httptrace.WithClientTrace(ctx, trace)

	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return nil, false, err
	}
	for k, vs := range headers {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}

	resp, err := client.Do(req)
	return resp, !wrote.Load(), err
}

// Classify reports whether err looks, from its own type, like a failure
// that happens before any bytes are written — DNS failure, dial failure, or
// a TLS/certificate failure. It exists as a cross-check against the
// httptrace-derived signal in Attempt: the two must agree, since a
// context.DeadlineExceeded during dial and one while waiting for response
// headers are indistinguishable as errors alone.
func Classify(err error) (beforeAnyWrite bool) {
	if err == nil {
		return false
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return true
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) && opErr.Op == "dial" {
		return true
	}
	var certVerifyErr *tls.CertificateVerificationError
	if errors.As(err, &certVerifyErr) {
		return true
	}
	var hostnameErr x509.HostnameError
	if errors.As(err, &hostnameErr) {
		return true
	}
	var unknownAuthErr x509.UnknownAuthorityError
	if errors.As(err, &unknownAuthErr) {
		return true
	}
	var certInvalidErr x509.CertificateInvalidError
	if errors.As(err, &certInvalidErr) {
		return true
	}
	return false
}

// RetryPolicy governs whether and how a failed attempt is retried.
// RetryTransportIfSent and RetryServerErrors are false for Init, Reverse,
// and Refund: a write is retried only when RequestNotSent proves nothing
// reached the gateway, never merely because the response was slow or a
// 500 came back, since either could mean the operation already happened.
// Query sets both true: it is read-only and safe to resend regardless of
// what came back.
type RetryPolicy struct {
	MaxRetries           int
	InitialBackoff       time.Duration
	MaxBackoff           time.Duration
	RetryTransportIfSent bool
	RetryServerErrors    bool
}

// Result is a completed attempt's response, fully read and closed.
type Result struct {
	StatusCode int
	Header     http.Header
	Body       []byte
	Attempts   int
}

// Send performs Attempt in a loop under policy, backing off between
// retries. ctx supplies the whole-call deadline; each attempt reuses it
// directly; so retries naturally get whatever time remains rather than a
// fresh budget per attempt.
func Send(ctx context.Context, client *http.Client, method, url string, body []byte, headers http.Header, policy RetryPolicy) (*Result, error) {
	for attempt := 0; ; attempt++ {
		resp, requestNotSent, err := Attempt(ctx, client, method, url, body, headers)
		if err != nil {
			retryable := attempt < policy.MaxRetries && (policy.RetryTransportIfSent || requestNotSent)
			if retryable && waitBackoff(ctx, policy, attempt, "") {
				continue
			}
			return nil, &Error{Err: err, RequestNotSent: requestNotSent}
		}

		data, readErr := io.ReadAll(io.LimitReader(resp.Body, MaxResponseBytes))
		resp.Body.Close()
		if readErr != nil {
			retryable := attempt < policy.MaxRetries && policy.RetryTransportIfSent
			if retryable && waitBackoff(ctx, policy, attempt, "") {
				continue
			}
			return nil, &Error{Err: readErr, RequestNotSent: false}
		}

		if policy.RetryServerErrors && resp.StatusCode >= 500 && attempt < policy.MaxRetries {
			if waitBackoff(ctx, policy, attempt, resp.Header.Get("Retry-After")) {
				continue
			}
			return nil, &Error{Err: ctx.Err(), RequestNotSent: false}
		}

		return &Result{StatusCode: resp.StatusCode, Header: resp.Header, Body: data, Attempts: attempt + 1}, nil
	}
}

// waitBackoff sleeps for the retry delay, racing it against ctx, and reports
// whether the wait completed (false means the deadline ran out first).
func waitBackoff(ctx context.Context, policy RetryPolicy, attempt int, retryAfter string) bool {
	delay := backoffDelay(policy, attempt)
	if ra := parseRetryAfter(retryAfter); ra > 0 {
		delay = ra
	}
	if delay <= 0 {
		return ctx.Err() == nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}

// backoffDelay is exponential from InitialBackoff, capped at MaxBackoff,
// with full jitter so a fleet does not retry in lockstep after a gateway
// blip.
func backoffDelay(policy RetryPolicy, attempt int) time.Duration {
	if policy.InitialBackoff <= 0 {
		return 0
	}
	if attempt > 20 {
		attempt = 20 // guard against overflow; the cap below dominates well before this
	}
	d := policy.InitialBackoff << attempt
	if policy.MaxBackoff > 0 && (d > policy.MaxBackoff || d < 0) {
		d = policy.MaxBackoff
	}
	if d <= 0 {
		return 0
	}
	return time.Duration(rand.Int63n(int64(d)) + 1)
}

// parseRetryAfter reads the Retry-After header, which the gateway may send
// as either a delay in seconds or an HTTP-date (RFC 9110 §10.2.3).
func parseRetryAfter(v string) time.Duration {
	if v == "" {
		return 0
	}
	if secs, err := strconv.Atoi(v); err == nil {
		if secs < 0 {
			return 0
		}
		return time.Duration(secs) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil {
		if d := time.Until(t); d > 0 {
			return d
		}
	}
	return 0
}
