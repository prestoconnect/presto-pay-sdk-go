package prestopay

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func newWebhookVerifier(t *testing.T, certPEM []byte, configure func(*WebhookConfig)) *WebhookVerifier {
	t.Helper()
	cfg := WebhookConfig{MerchantIDs: []string{"MID1"}, PrestoPublicKeys: [][]byte{certPEM}}
	if configure != nil {
		configure(&cfg)
	}
	v, err := NewWebhookVerifier(cfg)
	if err != nil {
		t.Fatalf("NewWebhookVerifier: %v", err)
	}
	return v
}

func validWebhookFields(ts time.Time) map[string]any {
	return map[string]any{
		"eventCode":     EventCodeAuthorised,
		"mid":           "MID1",
		"prestoMrn":     "PM1",
		"paymentRefNum": "PP1",
		"txnRefNum":     "order-123",
		"eventRefNum":   "evt-1",
		"eventTs":       FormatTimestamp(ts),
		"amount":        10_000,
		"currencyCode":  "MYR",
		"ts":            FormatTimestamp(ts),
		"success":       true,
	}
}

func TestWebhookVerifier_Success_AuthorisedTrue(t *testing.T) {
	privatePEM, certPEM := testMaterial(t)
	sign := gatewaySigner(t, privatePEM)
	now := time.Now()
	v := newWebhookVerifier(t, certPEM, func(cfg *WebhookConfig) { cfg.Now = func() time.Time { return now } })

	event, err := v.Verify(sign(validWebhookFields(now)))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if event.EventCode != EventCodeAuthorised || !event.Success {
		t.Fatalf("EventCode/Success = %q/%t, want %q/true", event.EventCode, event.Success, EventCodeAuthorised)
	}
	if event.EventRefNum != "evt-1" || event.Amount != 10_000 || event.PrestoMRN != "PM1" {
		t.Fatalf("event = %+v", event)
	}
}

// A webhook says what happened, not the payment's resulting status: the
// event carries eventCode and success exactly as sent and derives nothing,
// so a failed Refunded (which leaves the payment as it was) is reported as
// just that.
func TestWebhookVerifier_ReportsEventCodeAndSuccessAsSent(t *testing.T) {
	privatePEM, certPEM := testMaterial(t)
	sign := gatewaySigner(t, privatePEM)
	now := time.Now()
	v := newWebhookVerifier(t, certPEM, func(cfg *WebhookConfig) { cfg.Now = func() time.Time { return now } })

	for _, eventCode := range []string{EventCodeAuthorised, EventCodeRefunded, EventCodeReversed, EventCodeCancelled} {
		for _, success := range []bool{true, false} {
			fields := validWebhookFields(now)
			fields["eventCode"] = eventCode
			fields["success"] = success
			event, err := v.Verify(sign(fields))
			if err != nil {
				t.Fatalf("Verify(%s, %t): %v", eventCode, success, err)
			}
			if event.EventCode != eventCode || event.Success != success {
				t.Fatalf("EventCode/Success = %q/%t, want %q/%t", event.EventCode, event.Success, eventCode, success)
			}
		}
	}
}

func TestWebhookVerifier_InvalidSignature(t *testing.T) {
	_, certPEM := testMaterial(t)
	otherPrivatePEM, _ := testMaterial(t)
	sign := gatewaySigner(t, otherPrivatePEM)
	now := time.Now()
	v := newWebhookVerifier(t, certPEM, func(cfg *WebhookConfig) { cfg.Now = func() time.Time { return now } })

	_, err := v.Verify(sign(validWebhookFields(now)))
	var sigErr *SignatureError
	if !errors.As(err, &sigErr) {
		t.Fatalf("Verify() error = %v, want *SignatureError", err)
	}
}

func TestWebhookVerifier_UnknownMerchantID(t *testing.T) {
	privatePEM, certPEM := testMaterial(t)
	sign := gatewaySigner(t, privatePEM)
	now := time.Now()
	v := newWebhookVerifier(t, certPEM, func(cfg *WebhookConfig) { cfg.Now = func() time.Time { return now } })

	fields := validWebhookFields(now)
	fields["mid"] = "SOMEONE-ELSE"
	_, err := v.Verify(sign(fields))
	var sigErr *SignatureError
	if !errors.As(err, &sigErr) {
		t.Fatalf("Verify() error = %v, want *SignatureError for an unrecognized mid", err)
	}
}

func TestWebhookVerifier_StaleTimestampTooOld(t *testing.T) {
	privatePEM, certPEM := testMaterial(t)
	sign := gatewaySigner(t, privatePEM)
	now := time.Now()
	v := newWebhookVerifier(t, certPEM, func(cfg *WebhookConfig) { cfg.Now = func() time.Time { return now } })

	_, err := v.Verify(sign(validWebhookFields(now.Add(-20 * time.Minute))))
	var sigErr *SignatureError
	if !errors.As(err, &sigErr) {
		t.Fatalf("Verify() error = %v, want *SignatureError for a stale ts", err)
	}
}

func TestWebhookVerifier_TimestampTooFarInFuture(t *testing.T) {
	privatePEM, certPEM := testMaterial(t)
	sign := gatewaySigner(t, privatePEM)
	now := time.Now()
	v := newWebhookVerifier(t, certPEM, func(cfg *WebhookConfig) { cfg.Now = func() time.Time { return now } })

	_, err := v.Verify(sign(validWebhookFields(now.Add(20 * time.Minute))))
	var sigErr *SignatureError
	if !errors.As(err, &sigErr) {
		t.Fatalf("Verify() error = %v, want *SignatureError for a ts too far ahead", err)
	}
}

func TestWebhookVerifier_RedeliveryWithFreshTimestampStillVerifies(t *testing.T) {
	privatePEM, certPEM := testMaterial(t)
	sign := gatewaySigner(t, privatePEM)
	now := time.Now()
	v := newWebhookVerifier(t, certPEM, func(cfg *WebhookConfig) { cfg.Now = func() time.Time { return now } })

	// Simulates a redelivery: the same eventRefNum, still within the
	// 15-minute freshness window because Presto refreshes ts on every
	// retry attempt.
	fields := validWebhookFields(now.Add(-10 * time.Minute))
	event, err := v.Verify(sign(fields))
	if err != nil {
		t.Fatalf("a redelivery inside the freshness window should still verify: %v", err)
	}
	if event.EventRefNum != "evt-1" {
		t.Fatalf("EventRefNum = %q, want evt-1", event.EventRefNum)
	}
}

func TestWebhookVerifier_MalformedBody(t *testing.T) {
	_, certPEM := testMaterial(t)
	v := newWebhookVerifier(t, certPEM, nil)
	_, err := v.Verify([]byte("not json"))
	var re *ResponseError
	if !errors.As(err, &re) {
		t.Fatalf("Verify() error = %v, want *ResponseError", err)
	}
}

func TestWebhookVerifier_MissingRequiredField(t *testing.T) {
	privatePEM, certPEM := testMaterial(t)
	sign := gatewaySigner(t, privatePEM)
	now := time.Now()
	v := newWebhookVerifier(t, certPEM, func(cfg *WebhookConfig) { cfg.Now = func() time.Time { return now } })

	fields := validWebhookFields(now)
	delete(fields, "eventRefNum")
	_, err := v.Verify(sign(fields))
	var re *ResponseError
	if !errors.As(err, &re) {
		t.Fatalf("Verify() error = %v, want *ResponseError for a missing required field", err)
	}
}

func TestWebhookVerifier_VerifyRequest(t *testing.T) {
	privatePEM, certPEM := testMaterial(t)
	sign := gatewaySigner(t, privatePEM)
	now := time.Now()
	v := newWebhookVerifier(t, certPEM, func(cfg *WebhookConfig) { cfg.Now = func() time.Time { return now } })

	body := sign(validWebhookFields(now))
	req := httptest.NewRequest(http.MethodPost, "/presto/notify", bytes.NewReader(body))
	event, err := v.VerifyRequest(req)
	if err != nil {
		t.Fatalf("VerifyRequest: %v", err)
	}
	if event.PrestoMRN != "PM1" {
		t.Fatalf("event = %+v", event)
	}
}

func TestNewWebhookVerifier_ValidationErrors(t *testing.T) {
	_, certPEM := testMaterial(t)
	if _, err := NewWebhookVerifier(WebhookConfig{PrestoPublicKeys: [][]byte{certPEM}}); err == nil {
		t.Fatal("expected an error for missing MerchantIDs")
	}
	if _, err := NewWebhookVerifier(WebhookConfig{MerchantIDs: []string{"MID1"}}); err == nil {
		t.Fatal("expected an error for missing PrestoPublicKeys")
	}
}

func TestAckForError(t *testing.T) {
	sigErr := newSignatureError(OpWebhook, "webhook", "", false)
	if AckForError(sigErr) != AckOK {
		t.Fatal("a *SignatureError should get AckOK")
	}
	respErr := newResponseError(OpWebhook, "webhook", nil, errors.New("x"), false, nil)
	if AckForError(respErr) != AckOK {
		t.Fatal("a *ResponseError should get AckOK")
	}
	if AckForError(errors.New("transient")) != AckResend {
		t.Fatal("any other error should get AckResend")
	}
}

// A Query made inside the webhook handler that gets a bad or unsigned
// response is the merchant's failure, not the webhook's: answering AckOK would
// tell Presto to stop delivering an event the handler never processed.
func TestAckForError_OutboundCallFailureAsksForResend(t *testing.T) {
	sigErr := newSignatureError(OpQuery, "response", "", false)
	if AckForError(sigErr) != AckResend {
		t.Fatal("a response *SignatureError from Query should get AckResend")
	}
	respErr := newResponseError(OpQuery, "response", nil, errors.New("x"), false, nil)
	if AckForError(respErr) != AckResend {
		t.Fatal("a response *ResponseError from Query should get AckResend")
	}
	wrapped := fmt.Errorf("fulfil: %w", respErr)
	if AckForError(wrapped) != AckResend {
		t.Fatal("a wrapped response *ResponseError should get AckResend")
	}
}

func TestWriteAck(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteAck(rec, AckOK)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Body.String(); got != `{"resend":false}` {
		t.Fatalf(`body = %q, want {"resend":false}`, got)
	}

	rec2 := httptest.NewRecorder()
	WriteAck(rec2, AckResend)
	if got := rec2.Body.String(); got != `{"resend":true}` {
		t.Fatalf(`body = %q, want {"resend":true}`, got)
	}
}
