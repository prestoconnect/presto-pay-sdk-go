package prestopay

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
	"unicode/utf8"

	"github.com/prestoconnect/presto-pay-sdk-go/prestopay/internal/canonical"
	"github.com/prestoconnect/presto-pay-sdk-go/prestopay/internal/crypto"
	"github.com/prestoconnect/presto-pay-sdk-go/prestopay/internal/transport"
)

const userAgent = "presto-pay-sdk-go/" + Version

// send signs fields, posts them to path, and verifies and decodes the
// response. It adds mid and ts, so the caller passes only the operation's
// own fields. reconcile is attached to any error for which the operation
// counts as ambiguous (see ambiguousWrite); it is nil for Query.
//
// Retries reuse the same signed body on every attempt rather than
// re-signing per attempt: internal/transport.Send has no hook to rebuild the
// body mid-retry, and since retries only happen inside one call's deadline
// (tens of seconds), a ts that was fresh when first signed stays well inside
// the gateway's 15-minute window for the life of the retry loop.
func (c *Client) send(ctx context.Context, op Operation, path string, fields map[string]any, reconcile *ReconcileKey, policy transport.RetryPolicy) (map[string]any, []byte, error) {
	ctx, cancel := c.withDeadline(ctx)
	defer cancel()

	requestTs := c.now()
	fields["mid"] = c.config.MerchantID
	fields["ts"] = FormatTimestamp(requestTs)

	for k, v := range fields {
		if s, ok := v.(string); ok && !utf8.ValidString(s) {
			return nil, nil, &ConfigError{Op: op, Field: k, Err: errors.New("not valid UTF-8")}
		}
	}

	canonicalReq, err := canonical.Build(fields)
	if err != nil {
		return nil, nil, &ConfigError{Op: op, Err: err}
	}
	signature, err := crypto.Sign(c.privateKey, canonicalReq)
	if err != nil {
		return nil, nil, fmt.Errorf("prestopay: signing request: %w", err)
	}
	fields["signature"] = signature

	body, err := encodeBody(fields)
	if err != nil {
		return nil, nil, &ConfigError{Op: op, Err: err}
	}

	headers := http.Header{
		"Content-Type": {"application/json; charset=UTF-8"},
		"User-Agent":   {userAgent},
	}

	result, err := transport.Send(ctx, c.httpClient, http.MethodPost, c.baseURL+path, body, headers, policy)
	if err != nil {
		var terr *transport.Error
		requestNotSent := errors.As(err, &terr) && terr.RequestNotSent
		return nil, nil, newTransportError(op, err, requestNotSent, reconcile)
	}

	if result.StatusCode != http.StatusOK {
		return nil, result.Body, newAPIError(op, KindHTTP, result.StatusCode, "", "", result.Body, "", c.showErrorBodies, reconcile)
	}

	decoded, err := decodeBody(result.Body)
	if err != nil {
		return nil, result.Body, newResponseError(op, "response", result.Body, err, c.showErrorBodies, reconcile)
	}

	canonicalResp, err := canonical.Build(decoded)
	if err != nil {
		return nil, result.Body, newResponseError(op, "response", result.Body, err, c.showErrorBodies, reconcile)
	}
	signatureValue, _ := decoded["signature"].(string)
	if signatureValue == "" || !c.verifiesAgainstAnyPrestoKey(canonicalResp, signatureValue) {
		return nil, result.Body, newSignatureError(op, "response", canonicalResp, c.showErrorBodies)
	}

	success, ok := decoded["success"].(bool)
	if !ok {
		return nil, result.Body, newResponseError(op, "response", result.Body, errors.New(`missing or non-boolean "success" field`), c.showErrorBodies, reconcile)
	}
	if !success {
		return nil, result.Body, c.newBusinessError(op, result, decoded, canonicalResp, requestTs, reconcile)
	}

	return decoded, result.Body, nil
}

// writePolicy governs Init, Reverse, Refund, and Raw.Post: retried only when
// RequestNotSent proves nothing reached the gateway, never merely because
// the response was slow or a 500 came back, since either could mean the
// operation already happened. It shares Config.RetryReads's backoff numbers
// with readPolicy rather than inventing a second knob.
func (c *Client) writePolicy() transport.RetryPolicy {
	return transport.RetryPolicy{
		MaxRetries:     c.retryReads.MaxRetries,
		InitialBackoff: c.retryReads.InitialBackoff,
		MaxBackoff:     c.retryReads.MaxBackoff,
	}
}

// readPolicy governs Query, the one operation safe to resend regardless of
// what came back.
func (c *Client) readPolicy() transport.RetryPolicy {
	policy := c.writePolicy()
	policy.RetryTransportIfSent = true
	policy.RetryServerErrors = true
	return policy
}

func (c *Client) verifiesAgainstAnyPrestoKey(canonicalString, signature string) bool {
	for _, key := range c.prestoPublicKeys {
		if crypto.Verify(key, canonicalString, signature) == nil {
			return true
		}
	}
	return false
}

// newBusinessError builds the *APIError for a signed success:false response,
// attaching the canonical string on 1006/1007 (the gateway does not
// distinguish a malformed signature from a well-formed one that failed to
// verify) and the observed clock offset on 1005.
func (c *Client) newBusinessError(op Operation, result *transport.Result, decoded map[string]any, canonicalResp string, requestTs time.Time, reconcile *ReconcileKey) *APIError {
	code, _ := decoded["errorCode"].(string)
	message, _ := decoded["errorMessage"].(string)

	debugCanonical := ""
	if code == ErrorCodeInvalidSignature || code == ErrorCodeSignatureVerificationFailed {
		debugCanonical = canonicalResp
	}

	apiErr := newAPIError(op, KindBusiness, result.StatusCode, code, message, result.Body, debugCanonical, c.showErrorBodies, reconcile)
	if code == ErrorCodeClockSkew {
		if responseTs, ok := decoded["ts"].(string); ok {
			if parsed, err := ParseTimestamp(responseTs); err == nil {
				apiErr = apiErr.withClockOffset(parsed.Sub(requestTs))
			}
		}
	}
	return apiErr
}

// encodeBody renders fields as JSON with HTML escaping disabled — the
// canonical string uses decoded values, so escaping cannot break a
// signature, but a body that reads the same as what was signed is worth it
// when someone is debugging with a packet capture — and without the
// trailing newline json.Encoder appends.
func encodeBody(fields map[string]any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(fields); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// decodeBody parses a response body with UseNumber, so large integers round-
// trip through canonicalization exactly instead of being rounded by the
// default float64 decoding.
func decodeBody(body []byte) (map[string]any, error) {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var decoded map[string]any
	if err := dec.Decode(&decoded); err != nil {
		return nil, err
	}
	return decoded, nil
}

// RawAPI signs, sends, verifies and returns the parsed object for a gateway
// endpoint this SDK does not wrap yet, and exposes the signing and
// verification primitives on their own.
type RawAPI struct {
	client *Client
}

// Post signs and sends body to path under the client's configured mid, and
// returns the verified, decoded response. body must not include mid, ts or
// signature; Post adds them. The caller's endpoint is unclassified, so a
// failure after an uncertain send is always treated as ambiguous (OpRaw),
// and there is no known key to reconcile by.
func (r RawAPI) Post(ctx context.Context, path string, body map[string]any) (map[string]any, error) {
	decoded, _, err := r.client.send(ctx, OpRaw, path, body, nil, r.client.writePolicy())
	return decoded, err
}

// Sign signs canonicalString with this client's private key.
func (r RawAPI) Sign(canonicalString string) (string, error) {
	return crypto.Sign(r.client.privateKey, canonicalString)
}

// VerifyBody reports whether body's signature verifies against this
// client's configured Presto public key(s).
func (r RawAPI) VerifyBody(body map[string]any) (bool, error) {
	canonicalString, err := canonical.Build(body)
	if err != nil {
		return false, err
	}
	signature, _ := body["signature"].(string)
	return r.client.verifiesAgainstAnyPrestoKey(canonicalString, signature), nil
}
