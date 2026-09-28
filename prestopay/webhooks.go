package prestopay

import (
	"crypto/rsa"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/prestoconnect/presto-pay-sdk-go/prestopay/internal/canonical"
	"github.com/prestoconnect/presto-pay-sdk-go/prestopay/internal/crypto"
	"github.com/prestoconnect/presto-pay-sdk-go/prestopay/internal/keys"
)

const (
	defaultMaxTimestampAge = 15 * time.Minute
	maxWebhookBodyBytes    = 1 << 20 // 1 MiB
)

// NotifyEvent is a verified webhook event.
type NotifyEvent struct {
	MID            string          `json:"mid"`
	EventCode      string          `json:"eventCode"`
	PrestoMRN      string          `json:"prestoMrn"`
	PaymentRefNum  string          `json:"paymentRefNum"`
	TxnRefNum      string          `json:"txnRefNum"`
	EventRefNum    string          `json:"eventRefNum"`
	EventTs        string          `json:"eventTs"`
	Amount         int64           `json:"amount"`
	CurrencyCode   string          `json:"currencyCode"`
	Ts             string          `json:"ts"`
	Success        bool            `json:"success"`
	UserRefNum     string          `json:"userRefNum,omitempty"`
	AdditionalData string          `json:"additionalData,omitempty"`
	PaymentDetails []PaymentDetail `json:"paymentDetails,omitempty"`

	// PaymentStatus is derived: for EventCodeAuthorised it reflects Success
	// (PaymentStatusAuthorised or PaymentStatusFailed); for every other event
	// code it is the event code itself. Query remains the authoritative
	// source of payment state.
	PaymentStatus string `json:"paymentStatus"`
}

// WebhookConfig configures a WebhookVerifier.
type WebhookConfig struct {
	// MerchantIDs is the set of mid values this verifier accepts. Presto
	// signs webhooks for every merchant with the same key, so this check is
	// mandatory, not defensive.
	MerchantIDs []string

	PrestoPublicKeys [][]byte

	// MaxTimestampAge is the freshness window; zero means 15 minutes.
	// Widening or disabling it is safe only if the caller deduplicates by
	// EventRefNum.
	MaxTimestampAge time.Duration

	// ShowErrorBodies includes RawBody/Canonical in Error() strings, same as
	// Config.ShowErrorBodies.
	ShowErrorBodies bool

	// Now overrides the clock used for freshness checks; nil means time.Now.
	Now func() time.Time
}

// WebhookVerifier verifies Presto webhook deliveries. It holds no private
// key, so a webhook-only service configures nothing else and never holds
// signing material.
type WebhookVerifier struct {
	merchantIDs      map[string]struct{}
	prestoPublicKeys []*rsa.PublicKey
	maxTimestampAge  time.Duration
	showErrorBodies  bool
	now              func() time.Time
}

// NewWebhookVerifier builds a WebhookVerifier, loading and validating the
// Presto public keys eagerly.
func NewWebhookVerifier(cfg WebhookConfig) (*WebhookVerifier, error) {
	ids := make(map[string]struct{}, len(cfg.MerchantIDs))
	for _, id := range cfg.MerchantIDs {
		if id != "" {
			ids[id] = struct{}{}
		}
	}
	if len(ids) == 0 {
		return nil, &ConfigError{Op: OpConfig, Field: "MerchantIDs"}
	}
	if len(cfg.PrestoPublicKeys) == 0 {
		return nil, &ConfigError{Op: OpConfig, Field: "PrestoPublicKeys"}
	}
	publicKeys := make([]*rsa.PublicKey, 0, len(cfg.PrestoPublicKeys))
	for _, raw := range cfg.PrestoPublicKeys {
		pub, err := keys.LoadPrestoPublicKey(raw)
		if err != nil {
			return nil, &ConfigError{Op: OpConfig, Field: "PrestoPublicKeys", Err: err}
		}
		publicKeys = append(publicKeys, pub)
	}

	maxAge := cfg.MaxTimestampAge
	if maxAge <= 0 {
		maxAge = defaultMaxTimestampAge
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}

	return &WebhookVerifier{
		merchantIDs:      ids,
		prestoPublicKeys: publicKeys,
		maxTimestampAge:  maxAge,
		showErrorBodies:  cfg.ShowErrorBodies,
		now:              now,
	}, nil
}

// VerifyRequest reads and verifies r's body. It reads through
// http.MaxBytesReader, so a misbehaving sender cannot exhaust memory; it does
// not restore r.Body afterward.
func (v *WebhookVerifier) VerifyRequest(r *http.Request) (NotifyEvent, error) {
	body := http.MaxBytesReader(nil, r.Body, maxWebhookBodyBytes)
	defer body.Close()
	data, err := io.ReadAll(body)
	if err != nil {
		return NotifyEvent{}, newResponseError(OpWebhook, "webhook", nil, err, v.showErrorBodies, nil)
	}
	return v.Verify(data)
}

// Verify verifies a raw webhook body, already read (for example by a queue
// consumer, or a framework that has already buffered the request).
func (v *WebhookVerifier) Verify(body []byte) (NotifyEvent, error) {
	decoded, err := decodeBody(body)
	if err != nil {
		return NotifyEvent{}, newResponseError(OpWebhook, "webhook", body, err, v.showErrorBodies, nil)
	}

	canonicalStr, err := canonical.Build(decoded)
	if err != nil {
		return NotifyEvent{}, newResponseError(OpWebhook, "webhook", body, err, v.showErrorBodies, nil)
	}
	// Signature is checked before mid and freshness, since a forged body
	// fails here regardless of what it claims.
	signature, _ := decoded["signature"].(string)
	if signature == "" || !v.verifiesAgainstAnyPrestoKey(canonicalStr, signature) {
		return NotifyEvent{}, newSignatureError(OpWebhook, "webhook", canonicalStr, v.showErrorBodies)
	}

	success, ok := decoded["success"].(bool)
	if !ok {
		return NotifyEvent{}, newResponseError(OpWebhook, "webhook", body, errors.New(`missing or non-boolean "success" field`), v.showErrorBodies, nil)
	}

	mid := stringField(decoded, "mid")
	if _, allowed := v.merchantIDs[mid]; !allowed {
		return NotifyEvent{}, newSignatureError(OpWebhook, "webhook", canonicalStr, v.showErrorBodies)
	}

	ts := stringField(decoded, "ts")
	tsInstant, err := ParseTimestamp(ts)
	if err != nil {
		return NotifyEvent{}, newResponseError(OpWebhook, "webhook", body, fmt.Errorf("ts: %w", err), v.showErrorBodies, nil)
	}
	if age := v.now().Sub(tsInstant); age > v.maxTimestampAge || age < -v.maxTimestampAge {
		return NotifyEvent{}, newSignatureError(OpWebhook, "webhook", canonicalStr, v.showErrorBodies)
	}

	eventCode, err := requireStringField(OpWebhook, decoded, body, "eventCode", v.showErrorBodies, nil)
	if err != nil {
		return NotifyEvent{}, err
	}
	prestoMrn, err := requireStringField(OpWebhook, decoded, body, "prestoMrn", v.showErrorBodies, nil)
	if err != nil {
		return NotifyEvent{}, err
	}
	paymentRefNum, err := requireStringField(OpWebhook, decoded, body, "paymentRefNum", v.showErrorBodies, nil)
	if err != nil {
		return NotifyEvent{}, err
	}
	txnRefNum, err := requireStringField(OpWebhook, decoded, body, "txnRefNum", v.showErrorBodies, nil)
	if err != nil {
		return NotifyEvent{}, err
	}
	eventRefNum, err := requireStringField(OpWebhook, decoded, body, "eventRefNum", v.showErrorBodies, nil)
	if err != nil {
		return NotifyEvent{}, err
	}
	eventTs, err := requireStringField(OpWebhook, decoded, body, "eventTs", v.showErrorBodies, nil)
	if err != nil {
		return NotifyEvent{}, err
	}
	currencyCode, err := requireStringField(OpWebhook, decoded, body, "currencyCode", v.showErrorBodies, nil)
	if err != nil {
		return NotifyEvent{}, err
	}
	amount, err := intField(OpWebhook, decoded, body, "amount", v.showErrorBodies, nil)
	if err != nil {
		return NotifyEvent{}, err
	}
	paymentDetails, err := parseListField[PaymentDetail](OpWebhook, decoded, body, "paymentDetails", v.showErrorBodies, nil)
	if err != nil {
		return NotifyEvent{}, err
	}

	paymentStatus := eventCode
	if eventCode == EventCodeAuthorised {
		if success {
			paymentStatus = PaymentStatusAuthorised
		} else {
			paymentStatus = PaymentStatusFailed
		}
	}

	return NotifyEvent{
		MID:            mid,
		EventCode:      eventCode,
		PrestoMRN:      prestoMrn,
		PaymentRefNum:  paymentRefNum,
		TxnRefNum:      txnRefNum,
		EventRefNum:    eventRefNum,
		EventTs:        eventTs,
		Amount:         amount,
		CurrencyCode:   currencyCode,
		Ts:             ts,
		Success:        success,
		UserRefNum:     stringField(decoded, "userRefNum"),
		AdditionalData: stringField(decoded, "additionalData"),
		PaymentDetails: paymentDetails,
		PaymentStatus:  paymentStatus,
	}, nil
}

func (v *WebhookVerifier) verifiesAgainstAnyPrestoKey(canonicalString, signature string) bool {
	for _, key := range v.prestoPublicKeys {
		if crypto.Verify(key, canonicalString, signature) == nil {
			return true
		}
	}
	return false
}

// Ack is a reply to a webhook delivery.
type Ack struct{ resend bool }

var (
	// AckOK accepts the delivery: {"resend":false}.
	AckOK = Ack{resend: false}
	// AckResend asks Presto to redeliver: {"resend":true}.
	AckResend = Ack{resend: true}
)

// WriteAck writes ack as the HTTP response to a webhook delivery.
func WriteAck(w http.ResponseWriter, ack Ack) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"resend":%t}`, ack.resend)
}

// AckForError picks the reply for a failed verification. A *SignatureError
// or *ResponseError is permanent — a bad signature, a foreign mid, or a
// stale ts fails identically on every redelivery — so it is answered with
// AckOK rather than looping Presto's retry schedule. Anything else is
// treated as the caller's own transient failure and gets AckResend.
func AckForError(err error) Ack {
	var sigErr *SignatureError
	var respErr *ResponseError
	if errors.As(err, &sigErr) || errors.As(err, &respErr) {
		return AckOK
	}
	return AckResend
}
