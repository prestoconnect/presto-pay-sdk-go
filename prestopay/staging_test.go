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

// TestStagingSmoke calls Presto's real staging gateway. It is excluded from
// the default build entirely (build tag) and, even under -tags staging,
// still requires an explicit opt-in, since it needs real credentials and
// creates a real (if unauthorized) payment record on every run.
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

	_, err = client.Payments.Init(ctx, initReq)
	if err == nil {
		t.Fatal("Init with a duplicate TxnRefNum should fail with error 1203")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("duplicate Init error = %v (%T), want *APIError", err, err)
	}
	if apiErr.Kind != KindBusiness {
		t.Errorf("duplicate Init: Kind = %v, want KindBusiness", apiErr.Kind)
	}
	if apiErr.ErrorCode != ErrorCodeDuplicateTxnRefNum {
		t.Errorf("duplicate Init: ErrorCode = %q, want %q", apiErr.ErrorCode, ErrorCodeDuplicateTxnRefNum)
	}
	if !apiErr.MayHaveTakenEffect() {
		t.Error("duplicate Init: MayHaveTakenEffect() = false, want true (1203 means a record exists)")
	}
	key, ok := apiErr.ReconcileBy()
	if !ok || key.TxnRefNum != txnRefNum {
		t.Errorf("duplicate Init: ReconcileBy() = %+v, %v, want TxnRefNum=%q", key, ok, txnRefNum)
	}
}
