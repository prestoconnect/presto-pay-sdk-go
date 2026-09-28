//go:build staging

package prestopay

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"
)

// TestStagingSmoke calls Presto's real staging gateway: Init, an immediate
// Query, a duplicate Init confirming the gateway's idempotent-retry behavior
// (Init on an existing TxnRefNum returns that payment's current status
// rather than error 1203), a Reverse on that same still-PendingAuthorise
// payment (which cancels it rather than failing — there is nothing to
// reverse financially if nothing was ever paid), and a Refund attempt on a
// second, separate PendingAuthorise payment (which is correctly rejected
// with error 1227, since nothing has been paid to refund). It is excluded
// from the default build entirely (build tag) and, even under -tags
// staging, still requires an explicit opt-in, since it creates real
// (if unauthorized) payment records on every run.
func TestStagingSmoke(t *testing.T) {
	if os.Getenv("PRESTOPAY_STAGING_SMOKE") != "1" {
		t.Skip("set PRESTOPAY_STAGING_SMOKE=1 to run this test against the real Presto staging gateway")
	}

	cfg, err := ConfigFromEnv(os.Getenv)
	if err != nil {
		t.Fatalf("ConfigFromEnv: %v", err)
	}
	// Strict regardless of what the environment sets: staging is exactly
	// where an unconfirmed rule guessed wrong should fail loudly instead of
	// being tolerated.
	cfg.Strict = true

	client, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	prestoMRN := os.Getenv("PRESTO_MRN")
	if prestoMRN == "" {
		t.Fatal("PRESTO_MRN environment variable is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	txnRefNum := fmt.Sprintf("smoke-%d", time.Now().UnixNano())
	initReq := InitRequest{
		PrestoMRN:    prestoMRN,
		TxnType:      TxnTypeWebPay,
		TxnRefNum:    txnRefNum,
		DisplayDesc:  "Go SDK staging smoke test",
		Amount:       100,
		CurrencyCode: "MYR",
		NotifyURL:    "https://example.invalid/presto/notify",
		RedirectURL:  "https://example.invalid/presto/return/" + txnRefNum,
	}

	initRes, err := client.Payments.Init(ctx, initReq)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if initRes.PaymentURL == "" {
		t.Error("Init: PaymentURL is empty")
	}
	if initRes.PaymentStatus != PaymentStatusPendingAuthorise {
		t.Errorf("Init: PaymentStatus = %q, want %q", initRes.PaymentStatus, PaymentStatusPendingAuthorise)
	}

	queryRes, err := client.Payments.Query(ctx, QueryRequest{
		PrestoMRN: prestoMRN,
		TxnRefNum: txnRefNum,
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if queryRes.PaymentRefNum != initRes.PaymentRefNum {
		t.Errorf("Query: PaymentRefNum = %q, want %q (from Init)", queryRes.PaymentRefNum, initRes.PaymentRefNum)
	}
	if queryRes.PaymentStatus != PaymentStatusPendingAuthorise {
		t.Errorf("Query: PaymentStatus = %q, want %q", queryRes.PaymentStatus, PaymentStatusPendingAuthorise)
	}

	// Init on an existing TxnRefNum is idempotent: it always returns that
	// payment's current status (success:true) rather than error 1203,
	// regardless of what state the payment has since reached.
	dupRes, err := client.Payments.Init(ctx, initReq)
	if err != nil {
		t.Fatalf("duplicate Init (existing TxnRefNum) should succeed idempotently: %v", err)
	}
	if dupRes.PaymentRefNum != initRes.PaymentRefNum {
		t.Errorf("duplicate Init: PaymentRefNum = %q, want %q (same as the first Init)",
			dupRes.PaymentRefNum, initRes.PaymentRefNum)
	}
	if dupRes.PaymentStatus != queryRes.PaymentStatus {
		t.Errorf("duplicate Init: PaymentStatus = %q, want %q (the existing record's current status)",
			dupRes.PaymentStatus, queryRes.PaymentStatus)
	}

	// Reversing a payment that was never paid cancels it rather than
	// failing: there is nothing to reverse financially, so the gateway
	// treats it as a cancellation. (An earlier version of this test assumed
	// Reverse would be rejected here; real staging traffic showed otherwise.)
	reverseRefNum := fmt.Sprintf("smoke-rev-%d", time.Now().UnixNano())
	reverseRes, err := client.Payments.Reverse(ctx, ReverseRequest{
		PrestoMRN:      prestoMRN,
		PaymentRefNum:  initRes.PaymentRefNum,
		ReversalRefNum: reverseRefNum,
		Remark:         "staging smoke test",
	})
	if err != nil {
		t.Fatalf("Reverse on a PendingAuthorise payment should succeed (cancelling it): %v", err)
	}
	if reverseRes.PaymentRefNum != initRes.PaymentRefNum {
		t.Errorf("Reverse: PaymentRefNum = %q, want %q", reverseRes.PaymentRefNum, initRes.PaymentRefNum)
	}
	if reverseRes.PaymentStatus != PaymentStatusCancelled {
		t.Errorf("Reverse: PaymentStatus = %q, want %q", reverseRes.PaymentStatus, PaymentStatusCancelled)
	}

	// Refund has no such carve-out: a second, untouched PendingAuthorise
	// payment is correctly rejected, since nothing has been paid yet.
	txnRefNum2 := fmt.Sprintf("smoke-2-%d", time.Now().UnixNano())
	initRes2, err := client.Payments.Init(ctx, InitRequest{
		PrestoMRN:    prestoMRN,
		TxnType:      TxnTypeWebPay,
		TxnRefNum:    txnRefNum2,
		DisplayDesc:  "Go SDK staging smoke test (refund check)",
		Amount:       100,
		CurrencyCode: "MYR",
		NotifyURL:    "https://example.invalid/presto/notify",
		RedirectURL:  "https://example.invalid/presto/return/" + txnRefNum2,
	})
	if err != nil {
		t.Fatalf("Init (second payment, for the Refund check): %v", err)
	}

	refundRefNum := fmt.Sprintf("smoke-rfnd-%d", time.Now().UnixNano())
	_, err = client.Payments.Refund(ctx, RefundRequest{
		PrestoMRN:     prestoMRN,
		PaymentRefNum: initRes2.PaymentRefNum,
		RefundRefNum:  refundRefNum,
		Remark:        "staging smoke test",
	})
	if err == nil {
		t.Fatal("Refund on a PendingAuthorise payment should fail: nothing has been paid yet")
	}
	var refundErr *APIError
	if !errors.As(err, &refundErr) {
		t.Fatalf("Refund error = %v (%T), want *APIError", err, err)
	}
	if refundErr.Kind != KindBusiness {
		t.Errorf("Refund: Kind = %v, want KindBusiness", refundErr.Kind)
	}
	if refundErr.ErrorCode != ErrorCodeInvalidStatusForRefund {
		t.Errorf("Refund: ErrorCode = %q, want %q (%s)", refundErr.ErrorCode, ErrorCodeInvalidStatusForRefund, refundErr.ErrorMessage)
	}
	if refundErr.MayHaveTakenEffect() {
		t.Error("Refund rejected for invalid payment state should not be ambiguous: nothing happened")
	}
}
