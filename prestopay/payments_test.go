package prestopay

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/prestoconnect/presto-pay-sdk-go/prestopay/internal/canonical"
	"github.com/prestoconnect/presto-pay-sdk-go/prestopay/internal/crypto"
)

// gatewaySigner signs a response body the same way Presto does, using the
// same key pair validConfig(t) configures as both the merchant's private key
// and Presto's public key — one key pair is enough to exercise this SDK's
// own request pipeline without a second, unused one.
func gatewaySigner(t *testing.T, privatePEM []byte) func(fields map[string]any) []byte {
	t.Helper()
	key, err := LoadPrivateKey(privatePEM)
	if err != nil {
		t.Fatalf("LoadPrivateKey: %v", err)
	}
	return func(fields map[string]any) []byte {
		canonicalStr, err := canonical.Build(fields)
		if err != nil {
			t.Fatalf("canonical.Build: %v", err)
		}
		sig, err := crypto.Sign(key, canonicalStr)
		if err != nil {
			t.Fatalf("crypto.Sign: %v", err)
		}
		fields["signature"] = sig
		body, err := json.Marshal(fields)
		if err != nil {
			t.Fatalf("json.Marshal: %v", err)
		}
		return body
	}
}

func decodeRequestBody(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	data, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("reading request body: %v", err)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var decoded map[string]any
	if err := dec.Decode(&decoded); err != nil {
		t.Fatalf("decoding request body: %v", err)
	}
	return decoded
}

// testGateway builds a client wired to an httptest.Server, and a sign func
// the handler can use to produce valid responses under the same key pair.
func testGateway(t *testing.T, handler func(sign func(map[string]any) []byte, w http.ResponseWriter, r *http.Request), configure func(*Config)) (*Client, *httptest.Server) {
	t.Helper()
	privatePEM, certPEM := testMaterial(t)
	sign := gatewaySigner(t, privatePEM)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler(sign, w, r)
	}))
	t.Cleanup(server.Close)

	cfg := Config{
		MerchantID:       "MID1",
		PrivateKeyPEM:    privatePEM,
		PrestoPublicKeys: [][]byte{certPEM},
		BaseURL:          server.URL,
	}
	if configure != nil {
		configure(&cfg)
	}
	client, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return client, server
}

func validInitRequest() InitRequest {
	return InitRequest{
		PrestoMRN:    "PM1",
		TxnType:      TxnTypeWebPay,
		TxnRefNum:    "order-123",
		DisplayDesc:  "Order 123",
		Amount:       10_000,
		CurrencyCode: "MYR",
		NotifyURL:    "https://merchant.example/notify",
		RedirectURL:  "https://merchant.example/return",
	}
}

func writeJSON(w http.ResponseWriter, status int, body []byte) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.WriteHeader(status)
	w.Write(body)
}

func TestPayments_Init_Success(t *testing.T) {
	ctx := t.Context()
	client, _ := testGateway(t, func(sign func(map[string]any) []byte, w http.ResponseWriter, r *http.Request) {
		req := decodeRequestBody(t, r)
		resp := map[string]any{
			"success":              true,
			"ts":                   req["ts"],
			"errorCode":            "",
			"errorMessage":         "",
			"prestoMrn":            req["prestoMrn"],
			"txnRefNum":            req["txnRefNum"],
			"paymentRefNum":        "PP1",
			"paymentStatus":        PaymentStatusPendingAuthorise,
			"paymentUrl":           "https://hpp-staging.example/PM1/PP1",
			"userRefNum":           "",
			"amount":               req["amount"],
			"currencyCode":         req["currencyCode"],
			"paymentRequestDate":   "20250423104500.000",
			"paymentFinalisedDate": "",
			"additionalData":       nil,
		}
		writeJSON(w, http.StatusOK, sign(resp))
	}, nil)

	res, err := client.Payments.Init(ctx, validInitRequest())
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if res.PaymentRefNum != "PP1" || res.PaymentStatus != PaymentStatusPendingAuthorise {
		t.Fatalf("Init() = %+v", res)
	}
	if res.PaymentURL == "" {
		t.Fatal("PaymentURL should be populated")
	}
	if res.Amount != 10_000 {
		t.Fatalf("Amount = %d, want 10000", res.Amount)
	}
}

func TestPayments_Init_ValidationErrors(t *testing.T) {
	base := validInitRequest()

	cases := []struct {
		name   string
		break_ func(*InitRequest)
		field  string
	}{
		{"MissingPrestoMRN", func(r *InitRequest) { r.PrestoMRN = "" }, "PrestoMRN"},
		{"MissingTxnType", func(r *InitRequest) { r.TxnType = "" }, "TxnType"},
		{"MissingTxnRefNum", func(r *InitRequest) { r.TxnRefNum = "" }, "TxnRefNum"},
		{"MissingDisplayDesc", func(r *InitRequest) { r.DisplayDesc = "" }, "DisplayDesc"},
		{"QRValueAndPayerRefNum", func(r *InitRequest) { r.QRValue = "q"; r.PayerRefNum = "p" }, "PayerRefNum"},
		{"AmountWithoutCurrency", func(r *InitRequest) { r.CurrencyCode = "" }, "CurrencyCode"},
		{"WebPayWithoutRedirectURL", func(r *InitRequest) { r.RedirectURL = "" }, "RedirectURL"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := base
			tc.break_(&req)
			if err := validateInit(req, false); err == nil {
				t.Fatal("expected a validation error")
			} else {
				var ce *ConfigError
				if !errors.As(err, &ce) || ce.Field != tc.field {
					t.Fatalf("validateInit() error = %v, want *ConfigError{Field: %s}", err, tc.field)
				}
			}
		})
	}
}

func TestPayments_Init_BusinessError1201(t *testing.T) {
	ctx := t.Context()
	client, _ := testGateway(t, func(sign func(map[string]any) []byte, w http.ResponseWriter, r *http.Request) {
		req := decodeRequestBody(t, r)
		resp := map[string]any{
			"success": false, "ts": req["ts"], "errorCode": ErrorCodeInvalidInput, "errorMessage": "Invalid input.",
		}
		writeJSON(w, http.StatusOK, sign(resp))
	}, nil)

	_, err := client.Payments.Init(ctx, validInitRequest())
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Kind != KindBusiness || apiErr.ErrorCode != ErrorCodeInvalidInput {
		t.Fatalf("Init() error = %v, want *APIError{Kind: KindBusiness, ErrorCode: %q}", err, ErrorCodeInvalidInput)
	}
	if apiErr.MayHaveTakenEffect() {
		t.Fatal("a plain business error (not 1203) means nothing happened")
	}
}

func TestPayments_Init_DuplicateTxnRefNum1203(t *testing.T) {
	ctx := t.Context()
	client, _ := testGateway(t, func(sign func(map[string]any) []byte, w http.ResponseWriter, r *http.Request) {
		req := decodeRequestBody(t, r)
		resp := map[string]any{
			"success": false, "ts": req["ts"], "errorCode": ErrorCodeDuplicateTxnRefNum, "errorMessage": "duplicate",
		}
		writeJSON(w, http.StatusOK, sign(resp))
	}, nil)

	req := validInitRequest()
	_, err := client.Payments.Init(ctx, req)
	var pe Error
	if !errors.As(err, &pe) || !pe.MayHaveTakenEffect() {
		t.Fatalf("Init() error = %v, want MayHaveTakenEffect() == true", err)
	}
	key, ok := pe.ReconcileBy()
	if !ok || key.TxnRefNum != req.TxnRefNum {
		t.Fatalf("ReconcileBy() = %+v, %v, want TxnRefNum=%q", key, ok, req.TxnRefNum)
	}
}

func TestPayments_Init_HTTPServerErrorIsAmbiguous(t *testing.T) {
	ctx := t.Context()
	client, _ := testGateway(t, func(sign func(map[string]any) []byte, w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}, nil)

	_, err := client.Payments.Init(ctx, validInitRequest())
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Kind != KindHTTP || apiErr.HTTPStatus != http.StatusInternalServerError {
		t.Fatalf("Init() error = %v, want *APIError{Kind: KindHTTP, HTTPStatus: 500}", err)
	}
	if !apiErr.MayHaveTakenEffect() {
		t.Fatal("a 500 on Init must be ambiguous")
	}
}

func TestPayments_Init_ClockSkew1005(t *testing.T) {
	ctx := t.Context()
	const offset = 90 * time.Second
	client, _ := testGateway(t, func(sign func(map[string]any) []byte, w http.ResponseWriter, r *http.Request) {
		req := decodeRequestBody(t, r)
		requestTs, err := ParseTimestamp(req["ts"].(string))
		if err != nil {
			t.Fatalf("ParseTimestamp: %v", err)
		}
		resp := map[string]any{
			"success": false, "ts": FormatTimestamp(requestTs.Add(offset)),
			"errorCode": ErrorCodeClockSkew, "errorMessage": "exceeded validity period",
		}
		writeJSON(w, http.StatusOK, sign(resp))
	}, nil)

	_, err := client.Payments.Init(ctx, validInitRequest())
	var apiErr *APIError
	if !errors.As(err, &apiErr) || !apiErr.HasClockOffset {
		t.Fatalf("Init() error = %v, want an *APIError with HasClockOffset", err)
	}
	if apiErr.ClockOffset < offset-time.Second || apiErr.ClockOffset > offset+time.Second {
		t.Fatalf("ClockOffset = %v, want ~%v", apiErr.ClockOffset, offset)
	}
}

func TestPayments_Init_SignatureFailureCodeAttachesCanonical(t *testing.T) {
	ctx := t.Context()
	client, _ := testGateway(t, func(sign func(map[string]any) []byte, w http.ResponseWriter, r *http.Request) {
		req := decodeRequestBody(t, r)
		resp := map[string]any{
			"success": false, "ts": req["ts"], "errorCode": ErrorCodeSignatureVerificationFailed, "errorMessage": "bad sig",
		}
		writeJSON(w, http.StatusOK, sign(resp))
	}, nil)

	_, err := client.Payments.Init(ctx, validInitRequest())
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Canonical == "" {
		t.Fatalf("Init() error = %v, want an *APIError with Canonical populated for code %s", err, ErrorCodeSignatureVerificationFailed)
	}
}

func TestPayments_Init_MalformedResponseBody(t *testing.T) {
	ctx := t.Context()
	client, _ := testGateway(t, func(sign func(map[string]any) []byte, w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, []byte("not json"))
	}, nil)

	_, err := client.Payments.Init(ctx, validInitRequest())
	var re *ResponseError
	if !errors.As(err, &re) {
		t.Fatalf("Init() error = %v, want *ResponseError", err)
	}
}

func TestPayments_Init_EchoMismatch(t *testing.T) {
	ctx := t.Context()
	client, _ := testGateway(t, func(sign func(map[string]any) []byte, w http.ResponseWriter, r *http.Request) {
		req := decodeRequestBody(t, r)
		resp := map[string]any{
			"success": true, "ts": req["ts"], "errorCode": "", "errorMessage": "",
			"prestoMrn": "SOMEONE-ELSE", "txnRefNum": req["txnRefNum"],
			"paymentRefNum": "PP1", "paymentStatus": PaymentStatusPendingAuthorise,
		}
		writeJSON(w, http.StatusOK, sign(resp))
	}, nil)

	_, err := client.Payments.Init(ctx, validInitRequest())
	var re *ResponseError
	if !errors.As(err, &re) {
		t.Fatalf("Init() error = %v, want *ResponseError for an echo mismatch", err)
	}
	if !re.MayHaveTakenEffect() {
		t.Fatal("an echo mismatch on Init is ambiguous")
	}
}

func TestPayments_Query_RetriesOnServerErrorThenSucceeds(t *testing.T) {
	ctx := t.Context()
	attempts := 0
	client, _ := testGateway(t, func(sign func(map[string]any) []byte, w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		req := decodeRequestBody(t, r)
		resp := map[string]any{
			"success": true, "ts": req["ts"], "errorCode": "", "errorMessage": "",
			"prestoMrn": req["prestoMrn"], "txnRefNum": req["txnRefNum"],
			"paymentRefNum": "PP1", "paymentStatus": PaymentStatusAuthorised,
		}
		writeJSON(w, http.StatusOK, sign(resp))
	}, func(cfg *Config) {
		cfg.RetryReads = RetryReads{MaxRetries: 1, InitialBackoff: time.Millisecond, MaxBackoff: 5 * time.Millisecond}
	})

	res, err := client.Payments.Query(ctx, QueryRequest{PrestoMRN: "PM1", TxnRefNum: "order-123"})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2 (one failure, one retry)", attempts)
	}
	if res.PaymentRefNum != "PP1" {
		t.Fatalf("Query() = %+v", res)
	}
}

func TestPayments_Query_RequiresPaymentRefNumOrTxnRefNum(t *testing.T) {
	err := validateQuery(QueryRequest{PrestoMRN: "PM1"})
	var ce *ConfigError
	if !errors.As(err, &ce) {
		t.Fatalf("validateQuery() error = %v, want *ConfigError", err)
	}
}

func TestPayments_Reverse_Success(t *testing.T) {
	ctx := t.Context()
	client, _ := testGateway(t, func(sign func(map[string]any) []byte, w http.ResponseWriter, r *http.Request) {
		req := decodeRequestBody(t, r)
		resp := map[string]any{
			"success": true, "ts": req["ts"], "errorCode": "", "errorMessage": "",
			"prestoMrn": req["prestoMrn"], "paymentRefNum": req["paymentRefNum"],
			"prestoReversalRefNum": "PR1", "amount": req["amount"], "currencyCode": "MYR",
			"paymentStatus": PaymentStatusReversed,
		}
		writeJSON(w, http.StatusOK, sign(resp))
	}, nil)

	res, err := client.Payments.Reverse(ctx, ReverseRequest{
		PrestoMRN: "PM1", ReversalRefNum: "rev-1", PaymentRefNum: "PP1",
	})
	if err != nil {
		t.Fatalf("Reverse: %v", err)
	}
	if res.PaymentStatus != PaymentStatusReversed {
		t.Fatalf("Reverse() = %+v", res)
	}
}

func TestPayments_Refund_Success(t *testing.T) {
	ctx := t.Context()
	client, _ := testGateway(t, func(sign func(map[string]any) []byte, w http.ResponseWriter, r *http.Request) {
		req := decodeRequestBody(t, r)
		resp := map[string]any{
			"success": true, "ts": req["ts"], "errorCode": "", "errorMessage": "",
			"prestoMrn": req["prestoMrn"], "paymentRefNum": req["paymentRefNum"],
			"prestoRefundRefNum": "PF1", "amount": 10_000, "refundAmount": req["amount"],
			"currencyCode": "MYR", "paymentStatus": PaymentStatusRefunded, "refundedDate": "20250423104500.000",
		}
		writeJSON(w, http.StatusOK, sign(resp))
	}, nil)

	res, err := client.Payments.Refund(ctx, RefundRequest{
		PrestoMRN: "PM1", PaymentRefNum: "PP1", RefundRefNum: "rf-1", Remark: "requested by customer", Amount: 5_000,
	})
	if err != nil {
		t.Fatalf("Refund: %v", err)
	}
	if res.RefundAmount != 5_000 || res.PaymentStatus != PaymentStatusRefunded {
		t.Fatalf("Refund() = %+v", res)
	}
}

// TestPayments_Init_OutgoingCanonicalMatchesCapturedExample drives a real
// Init call through the full pipeline (validation, field building,
// canonicalization, signing) with the same field values as a worked example
// captured from Presto, and checks the canonical string the mock gateway
// receives against the one Presto's own signature was confirmed to verify
// against — the same vector internal/canonical's tests check in isolation,
// exercised here end to end.
func TestPayments_Init_OutgoingCanonicalMatchesCapturedExample(t *testing.T) {
	ctx := t.Context()
	const wantCanonical = "1200:MYR:Order #12345:PW2401XH9KCX:https://merchant.example.com/webhook/notify:" +
		"PM240110XDSFC:https://merchant.example.com/redirect/TXN10001:20250423104500.000:TXN10001:WebPay"

	fixedTs, err := ParseTimestamp("20250423104500.000")
	if err != nil {
		t.Fatalf("ParseTimestamp: %v", err)
	}

	var gotCanonical string
	client, _ := testGateway(t, func(sign func(map[string]any) []byte, w http.ResponseWriter, r *http.Request) {
		req := decodeRequestBody(t, r)
		gotCanonical, err = canonical.Build(req)
		if err != nil {
			t.Fatalf("canonical.Build on the received request: %v", err)
		}
		resp := map[string]any{
			"success": true, "ts": req["ts"], "errorCode": "", "errorMessage": "",
			"prestoMrn": req["prestoMrn"], "txnRefNum": req["txnRefNum"],
			"paymentRefNum": "PP1", "paymentStatus": PaymentStatusPendingAuthorise,
		}
		writeJSON(w, http.StatusOK, sign(resp))
	}, func(cfg *Config) {
		cfg.MerchantID = "PW2401XH9KCX"
		cfg.Now = func() time.Time { return fixedTs }
	})

	_, err = client.Payments.Init(ctx, InitRequest{
		PrestoMRN:    "PM240110XDSFC",
		TxnType:      TxnTypeWebPay,
		TxnRefNum:    "TXN10001",
		DisplayDesc:  "Order #12345",
		Amount:       1200,
		CurrencyCode: "MYR",
		NotifyURL:    "https://merchant.example.com/webhook/notify",
		RedirectURL:  "https://merchant.example.com/redirect/TXN10001",
	})
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if gotCanonical != wantCanonical {
		t.Fatalf("outgoing canonical string =\n%q\nwant\n%q", gotCanonical, wantCanonical)
	}
}

func TestRawAPI_SignVerifyPost(t *testing.T) {
	ctx := t.Context()
	client, _ := testGateway(t, func(sign func(map[string]any) []byte, w http.ResponseWriter, r *http.Request) {
		req := decodeRequestBody(t, r)
		resp := map[string]any{
			"success": true, "ts": req["ts"], "errorCode": "", "errorMessage": "", "prestoMrn": req["prestoMrn"],
		}
		writeJSON(w, http.StatusOK, sign(resp))
	}, nil)

	sig, err := client.Raw.Sign("a:b:c")
	if err != nil || sig == "" {
		t.Fatalf("Raw.Sign() = %q, %v", sig, err)
	}

	decoded, err := client.Raw.Post(ctx, "/v1/ext/some/endpoint", map[string]any{"prestoMrn": "PM1"})
	if err != nil {
		t.Fatalf("Raw.Post: %v", err)
	}
	ok, err := client.Raw.VerifyBody(decoded)
	if err != nil || !ok {
		t.Fatalf("Raw.VerifyBody() = %v, %v, want true, nil", ok, err)
	}
}
