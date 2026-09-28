package prestopay

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestTransportError_QueryNeverAmbiguous(t *testing.T) {
	e := newTransportError(OpQuery, context.DeadlineExceeded, false, nil)
	if e.MayHaveTakenEffect() {
		t.Fatal("Query transport error should never be ambiguous")
	}
	if _, ok := e.ReconcileBy(); ok {
		t.Fatal("Query transport error should not carry a reconcile key")
	}
}

func TestTransportError_WriteAmbiguousOnlyWhenSent(t *testing.T) {
	rk := ReconcileKey{TxnRefNum: "TXN1"}

	notSent := newTransportError(OpInit, errors.New("dial tcp: connect: connection refused"), true, &rk)
	if notSent.MayHaveTakenEffect() {
		t.Fatal("RequestNotSent=true must mean not ambiguous")
	}
	if _, ok := notSent.ReconcileBy(); ok {
		t.Fatal("a definitely-not-sent request should not carry a reconcile key")
	}

	sent := newTransportError(OpInit, context.DeadlineExceeded, false, &rk)
	if !sent.MayHaveTakenEffect() {
		t.Fatal("RequestNotSent=false on Init must be ambiguous")
	}
	key, ok := sent.ReconcileBy()
	if !ok || key != rk {
		t.Fatalf("ReconcileBy() = %+v, %v, want %+v, true", key, ok, rk)
	}
}

func TestTransportError_Unwrap(t *testing.T) {
	e := newTransportError(OpQuery, context.DeadlineExceeded, false, nil)
	if !errors.Is(e, context.DeadlineExceeded) {
		t.Fatal("errors.Is should see through TransportError to the wrapped error")
	}
}

func TestAPIError_HTTPStatusAmbiguity(t *testing.T) {
	rk := ReconcileKey{PaymentRefNum: "PP1"}

	for _, tc := range []struct {
		op     Operation
		status int
		want   bool
	}{
		{OpInit, 500, true},
		{OpInit, 503, true},
		{OpInit, 400, false},
		{OpQuery, 500, false}, // Query is never ambiguous
		{OpReverse, 500, true},
		{OpRefund, 500, true},
	} {
		e := newAPIError(tc.op, KindHTTP, tc.status, "", "", nil, "", false, &rk)
		if got := e.MayHaveTakenEffect(); got != tc.want {
			t.Errorf("op=%v status=%d: MayHaveTakenEffect() = %v, want %v", tc.op, tc.status, got, tc.want)
		}
	}
}

func TestAPIError_1203MeansRecordExists(t *testing.T) {
	rk := ReconcileKey{TxnRefNum: "TXN1"}
	e := newAPIError(OpInit, KindBusiness, 200, "1203", "duplicate txnRefNum", nil, "", false, &rk)
	if !e.MayHaveTakenEffect() {
		t.Fatal("1203 must be ambiguous: a payment record already exists")
	}
	key, ok := e.ReconcileBy()
	if !ok || key != rk {
		t.Fatalf("ReconcileBy() = %+v, %v, want %+v, true", key, ok, rk)
	}
}

func TestAPIError_OtherBusinessErrorsAreNotAmbiguous(t *testing.T) {
	e := newAPIError(OpInit, KindBusiness, 200, "1201", "invalid input", nil, "", false, nil)
	if e.MayHaveTakenEffect() {
		t.Fatal("a plain business error (not 1203) means nothing happened")
	}
}

func TestAPIError_BodyRedactedByDefault(t *testing.T) {
	e := newAPIError(OpInit, KindBusiness, 200, "1201", "invalid input", []byte(`{"cardBin":"411111"}`), "", false, nil)
	if got := e.Error(); containsBody(got, "cardBin") {
		t.Fatalf("Error() leaked the raw body when showBody is false: %q", got)
	}
	if string(e.RawBody) == "" {
		t.Fatal("RawBody must always be populated even when Error() redacts it")
	}
}

func TestAPIError_BodyShownWhenRequested(t *testing.T) {
	e := newAPIError(OpInit, KindBusiness, 200, "1201", "invalid input", []byte(`{"cardBin":"411111"}`), "", true, nil)
	if got := e.Error(); !containsBody(got, "cardBin") {
		t.Fatalf("Error() should include the body when showBody is true: %q", got)
	}
}

func TestAPIError_ClockOffset(t *testing.T) {
	e := newAPIError(OpInit, KindBusiness, 200, "1005", "exceeded validity period", nil, "", false, nil).
		withClockOffset(90 * time.Second)
	if !e.HasClockOffset || e.ClockOffset != 90*time.Second {
		t.Fatalf("clock offset not recorded: %+v", e)
	}
	if got := e.Error(); !containsBody(got, "1m30s") {
		t.Fatalf("Error() should mention the clock offset: %q", got)
	}
}

func TestSignatureError_WebhookNeverAmbiguous(t *testing.T) {
	e := newSignatureError(OpWebhook, "webhook", "a:b:c", false)
	if e.MayHaveTakenEffect() {
		t.Fatal("a webhook signature failure reports on an event, not a request this SDK made")
	}
}

func TestSignatureError_ResponseAmbiguousForWrites(t *testing.T) {
	e := newSignatureError(OpReverse, "response", "a:b:c", false)
	if !e.MayHaveTakenEffect() {
		t.Fatal("a response signature failure on a write operation is ambiguous: the request may have reached the gateway before the response failed to verify")
	}
}

func TestSignatureError_ResponseNotAmbiguousForQuery(t *testing.T) {
	e := newSignatureError(OpQuery, "response", "a:b:c", false)
	if e.MayHaveTakenEffect() {
		t.Fatal("Query is read-only; a signature failure there means nothing happened")
	}
}

func TestResponseError_EchoMismatchAmbiguousForWrites(t *testing.T) {
	rk := ReconcileKey{TxnRefNum: "TXN1"}
	e := newResponseError(OpInit, "response", []byte(`{}`), errors.New("echo mismatch"), false, &rk)
	if !e.MayHaveTakenEffect() {
		t.Fatal("an echo mismatch on Init is ambiguous")
	}
	if key, ok := e.ReconcileBy(); !ok || key != rk {
		t.Fatalf("ReconcileBy() = %+v, %v, want %+v, true", key, ok, rk)
	}
}

func TestResponseError_Unwrap(t *testing.T) {
	inner := errors.New("bad json")
	e := newResponseError(OpQuery, "response", nil, inner, false, nil)
	if !errors.Is(e, inner) {
		t.Fatal("errors.Is should see through ResponseError to the wrapped error")
	}
}

func TestTransportError_RawAmbiguousOnlyWhenSent(t *testing.T) {
	notSent := newTransportError(OpRaw, errors.New("dial tcp: connect: connection refused"), true, nil)
	if notSent.MayHaveTakenEffect() {
		t.Fatal("RequestNotSent=true must mean not ambiguous")
	}

	sent := newTransportError(OpRaw, context.DeadlineExceeded, false, nil)
	if !sent.MayHaveTakenEffect() {
		t.Fatal("Raw is unclassified, so RequestNotSent=false must be treated as ambiguous")
	}
}

func TestAllErrorTypesImplementError(t *testing.T) {
	var errs []Error
	errs = append(errs,
		&ConfigError{Op: OpConfig, Field: "MerchantID"},
		newTransportError(OpQuery, errors.New("boom"), false, nil),
		newAPIError(OpQuery, KindHTTP, 500, "", "", nil, "", false, nil),
		newSignatureError(OpQuery, "response", "", false),
		newResponseError(OpQuery, "response", nil, errors.New("boom"), false, nil),
	)
	for _, e := range errs {
		if e.Error() == "" {
			t.Errorf("%T: Error() returned empty string", e)
		}
		_ = e.Operation()
		_ = e.MayHaveTakenEffect()
		_, _ = e.ReconcileBy()
	}
}

func containsBody(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
