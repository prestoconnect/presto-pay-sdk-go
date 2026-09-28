package prestopay

import (
	"fmt"
	"time"
)

// Error is implemented by every error type this package returns from a
// network or verification call.
type Error interface {
	error
	Operation() Operation
	MayHaveTakenEffect() bool
	ReconcileBy() (ReconcileKey, bool)
}

// Kind distinguishes an HTTP-level API error from a business error, which
// share the *APIError type because both carry a signed, verified body.
type Kind int

const (
	KindHTTP Kind = iota
	KindBusiness
)

func (k Kind) String() string {
	if k == KindBusiness {
		return "business"
	}
	return "http"
}

// ReconcileKey identifies the payment to look up with Query after an
// ambiguous failure on Init, Reverse, or Refund.
type ReconcileKey struct {
	TxnRefNum     string
	PaymentRefNum string
}

// ambiguousWrite reports whether op is one of the operations for which an
// ambiguous failure means the request may have reached the gateway (Init,
// Reverse, Refund, and Raw, whose caller-chosen endpoint this SDK cannot
// classify as idempotent). Query is read-only and safe to retry, so a
// failure there always means nothing happened; the same is true of Webhook
// and Config, which never send a request at all.
func ambiguousWrite(op Operation) bool {
	switch op {
	case OpInit, OpReverse, OpRefund, OpRaw:
		return true
	default:
		return false
	}
}

func opName(op Operation) string {
	switch op {
	case OpInit:
		return "init"
	case OpQuery:
		return "query"
	case OpReverse:
		return "reverse"
	case OpRefund:
		return "refund"
	case OpWebhook:
		return "webhook"
	case OpConfig:
		return "config"
	case OpRaw:
		return "raw"
	default:
		return "unknown"
	}
}

// ConfigError reports invalid configuration or request input, including
// invalid UTF-8 in an outgoing field.
type ConfigError struct {
	Op    Operation
	Field string
	Err   error
}

func (e *ConfigError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("prestopay: %s: invalid %s: %v", opName(e.Op), e.Field, e.Err)
	}
	return fmt.Sprintf("prestopay: %s: invalid %s", opName(e.Op), e.Field)
}

func (e *ConfigError) Unwrap() error                     { return e.Err }
func (e *ConfigError) Operation() Operation              { return e.Op }
func (e *ConfigError) MayHaveTakenEffect() bool          { return false }
func (e *ConfigError) ReconcileBy() (ReconcileKey, bool) { return ReconcileKey{}, false }

// TransportError reports a network failure, timeout, or context
// cancellation while calling the gateway.
type TransportError struct {
	Op  Operation
	Err error

	// RequestNotSent is true only when an httptrace.ClientTrace proved that
	// no byte of the request reached the socket. It is never inferred from
	// the error's type alone.
	RequestNotSent bool

	tookEffect bool
	reconcile  *ReconcileKey
}

// newTransportError decides MayHaveTakenEffect at construction: only here is
// it known which operation ran. Query is safe to resend regardless of
// RequestNotSent, so it never counts as ambiguous.
func newTransportError(op Operation, err error, requestNotSent bool, reconcile *ReconcileKey) *TransportError {
	e := &TransportError{Op: op, Err: err, RequestNotSent: requestNotSent}
	if ambiguousWrite(op) && !requestNotSent {
		e.tookEffect = true
		e.reconcile = reconcile
	}
	return e
}

func (e *TransportError) Error() string {
	return fmt.Sprintf("prestopay: %s: transport error (request sent: %v): %v", opName(e.Op), !e.RequestNotSent, e.Err)
}

func (e *TransportError) Unwrap() error            { return e.Err }
func (e *TransportError) Operation() Operation     { return e.Op }
func (e *TransportError) MayHaveTakenEffect() bool { return e.tookEffect }
func (e *TransportError) ReconcileBy() (ReconcileKey, bool) {
	if e.reconcile == nil {
		return ReconcileKey{}, false
	}
	return *e.reconcile, true
}

// APIError reports a non-200 HTTP status (Kind: KindHTTP) or a signed
// success:false response (Kind: KindBusiness).
type APIError struct {
	Op           Operation
	Kind         Kind
	HTTPStatus   int    // set for KindHTTP
	ErrorCode    string // set for KindBusiness
	ErrorMessage string // set for KindBusiness
	RawBody      []byte

	// Canonical is set when ErrorCode is 1006 or 1007: the gateway does not
	// distinguish a malformed signature from a well-formed one that failed
	// to verify, so both get this field for debugging.
	Canonical string

	// ClockOffset is the observed difference between this host's ts and the
	// gateway's, set when ErrorCode is 1005.
	ClockOffset    time.Duration
	HasClockOffset bool

	// showBody controls whether Error() includes RawBody/Canonical, which
	// can carry PII (cardBin, cardSummary, receiptEmail, receiptName). The
	// fields themselves are always populated; this only affects the string.
	showBody bool

	tookEffect bool
	reconcile  *ReconcileKey
}

func (e *APIError) Error() string {
	switch e.Kind {
	case KindBusiness:
		msg := fmt.Sprintf("prestopay: %s: business error %s: %s", opName(e.Op), e.ErrorCode, e.ErrorMessage)
		if e.HasClockOffset {
			msg += fmt.Sprintf(" (observed clock offset %v)", e.ClockOffset)
		}
		if e.showBody {
			msg += fmt.Sprintf(" body=%s", e.RawBody)
		}
		if e.Canonical != "" && e.showBody {
			msg += fmt.Sprintf(" canonical=%q", e.Canonical)
		}
		return msg
	default:
		msg := fmt.Sprintf("prestopay: %s: http status %d", opName(e.Op), e.HTTPStatus)
		if e.showBody {
			msg += fmt.Sprintf(" body=%s", e.RawBody)
		}
		return msg
	}
}

func (e *APIError) Operation() Operation     { return e.Op }
func (e *APIError) MayHaveTakenEffect() bool { return e.tookEffect }
func (e *APIError) ReconcileBy() (ReconcileKey, bool) {
	if e.reconcile == nil {
		return ReconcileKey{}, false
	}
	return *e.reconcile, true
}

// SignatureError reports a missing or invalid signature, a webhook whose
// mid does not match a configured merchant, or a webhook ts outside the
// freshness window.
type SignatureError struct {
	Op     Operation
	Source string // "response" or "webhook"

	// Canonical is the string that failed to verify, for debugging.
	Canonical string
	showBody  bool

	tookEffect bool
}

func (e *SignatureError) Error() string {
	msg := fmt.Sprintf("prestopay: %s: signature error (%s)", opName(e.Op), e.Source)
	if e.showBody && e.Canonical != "" {
		msg += fmt.Sprintf(" canonical=%q", e.Canonical)
	}
	return msg
}

func (e *SignatureError) Operation() Operation              { return e.Op }
func (e *SignatureError) MayHaveTakenEffect() bool          { return e.tookEffect }
func (e *SignatureError) ReconcileBy() (ReconcileKey, bool) { return ReconcileKey{}, false }

// ResponseError reports a malformed body, a missing required field, a
// malformed ts, or an echo mismatch (the response's prestoMrn/txnRefNum
// does not match what was signed into the request).
type ResponseError struct {
	Op      Operation
	Source  string // "response" or "webhook"
	RawBody []byte
	Err     error

	showBody bool

	tookEffect bool
	reconcile  *ReconcileKey
}

func (e *ResponseError) Error() string {
	msg := fmt.Sprintf("prestopay: %s: malformed response (%s): %v", opName(e.Op), e.Source, e.Err)
	if e.showBody {
		msg += fmt.Sprintf(" body=%s", e.RawBody)
	}
	return msg
}

func (e *ResponseError) Unwrap() error            { return e.Err }
func (e *ResponseError) Operation() Operation     { return e.Op }
func (e *ResponseError) MayHaveTakenEffect() bool { return e.tookEffect }
func (e *ResponseError) ReconcileBy() (ReconcileKey, bool) {
	if e.reconcile == nil {
		return ReconcileKey{}, false
	}
	return *e.reconcile, true
}

// newAPIError decides MayHaveTakenEffect at construction: a KindHTTP status
// of 500 or above is ambiguous for a write operation; a KindBusiness error
// is ambiguous only for code 1203, which means a payment record already
// exists.
func newAPIError(op Operation, kind Kind, httpStatus int, code, message string, raw []byte, canonical string, showBody bool, reconcile *ReconcileKey) *APIError {
	e := &APIError{
		Op: op, Kind: kind, HTTPStatus: httpStatus, ErrorCode: code, ErrorMessage: message,
		RawBody: raw, Canonical: canonical, showBody: showBody,
	}
	switch kind {
	case KindHTTP:
		e.tookEffect = httpStatus >= 500 && ambiguousWrite(op)
	case KindBusiness:
		e.tookEffect = code == "1203"
	}
	if e.tookEffect {
		e.reconcile = reconcile
	}
	return e
}

func (e *APIError) withClockOffset(d time.Duration) *APIError {
	e.ClockOffset = d
	e.HasClockOffset = true
	return e
}

// newSignatureError decides MayHaveTakenEffect at construction: a response
// signature failure is ambiguous for a write operation, but a webhook
// signature failure is not — it reports on an event that already happened
// rather than on a request this SDK made.
func newSignatureError(op Operation, source, canonical string, showBody bool) *SignatureError {
	return &SignatureError{
		Op: op, Source: source, Canonical: canonical, showBody: showBody,
		tookEffect: source == "response" && ambiguousWrite(op),
	}
}

// newResponseError decides MayHaveTakenEffect the same way as
// newSignatureError: ambiguous only for a write operation's own response,
// never for a webhook.
func newResponseError(op Operation, source string, raw []byte, err error, showBody bool, reconcile *ReconcileKey) *ResponseError {
	e := &ResponseError{Op: op, Source: source, RawBody: raw, Err: err, showBody: showBody}
	e.tookEffect = source == "response" && ambiguousWrite(op)
	if e.tookEffect {
		e.reconcile = reconcile
	}
	return e
}
