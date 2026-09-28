// Command net-http is a runnable demo of the Go Presto Pay SDK against
// Presto's real staging gateway, using only net/http and the SDK itself —
// no third-party dependencies.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/prestoconnect/presto-pay-sdk-go/prestopay"
)

func main() {
	loadDotEnv(".env")

	cfg, err := prestopay.ConfigFromEnv(os.Getenv)
	if err != nil {
		log.Fatalf("ConfigFromEnv: %v", err)
	}
	client, err := prestopay.New(cfg)
	if err != nil {
		log.Fatalf("New: %v", err)
	}

	verifier, err := prestopay.NewWebhookVerifier(prestopay.WebhookConfig{
		MerchantIDs:      []string{cfg.MerchantID},
		PrestoPublicKeys: cfg.PrestoPublicKeys,
	})
	if err != nil {
		log.Fatalf("NewWebhookVerifier: %v", err)
	}

	prestoMRN := os.Getenv("PRESTO_MRN")
	if prestoMRN == "" {
		log.Fatal("PRESTO_MRN environment variable is required")
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	baseURL := os.Getenv("PUBLIC_URL")
	if baseURL == "" {
		baseURL = "http://localhost:" + port
		log.Printf("WARNING: PUBLIC_URL is not set; using %s for redirects and notifyUrl. Presto cannot "+
			"deliver webhooks to localhost. Set PUBLIC_URL after exposing this port publicly to enable "+
			"webhook delivery.", baseURL)
	}

	deliveries := newDeliveryLog()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /", handleHome)
	mux.HandleFunc("POST /checkout", handleCheckout(client, prestoMRN, baseURL))
	mux.HandleFunc("GET /payments/{paymentRefNum}", handleQuery(client, prestoMRN))
	mux.HandleFunc("POST /payments/{paymentRefNum}/reverse", handleReverse(client, prestoMRN))
	mux.HandleFunc("POST /payments/{paymentRefNum}/refund", handleRefund(client, prestoMRN))
	mux.HandleFunc("POST /presto/notify", handleNotify(verifier, deliveries))

	log.Printf("Presto Pay SDK demo listening on http://localhost:%s", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}

func handleHome(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "Presto Pay SDK Go demo is running. See README.md for the route table and curl examples.")
}

func handleCheckout(client *prestopay.Client, prestoMRN, baseURL string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			TxnRefNum    string `json:"txnRefNum"`
			Amount       int64  `json:"amount"`
			CurrencyCode string `json:"currencyCode"`
		}
		if r.Body != nil {
			_ = json.NewDecoder(r.Body).Decode(&input) // best-effort; zero values below fill the rest
		}
		if input.TxnRefNum == "" {
			input.TxnRefNum = fmt.Sprintf("order-%d", time.Now().UnixNano())
		}
		if input.CurrencyCode == "" {
			input.CurrencyCode = "MYR"
		}
		if input.Amount == 0 {
			input.Amount = 1000
		}

		res, err := client.Payments.Init(r.Context(), prestopay.InitRequest{
			PrestoMRN:    prestoMRN,
			TxnType:      prestopay.TxnTypeWebPay,
			TxnRefNum:    input.TxnRefNum,
			DisplayDesc:  "Go SDK demo order " + input.TxnRefNum,
			Amount:       input.Amount,
			CurrencyCode: input.CurrencyCode,
			NotifyURL:    baseURL + "/presto/notify",
			RedirectURL:  baseURL + "/return/" + input.TxnRefNum,
		})
		if err != nil {
			writePrestoError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	}
}

func handleQuery(client *prestopay.Client, prestoMRN string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		res, err := client.Payments.Query(r.Context(), prestopay.QueryRequest{
			PrestoMRN:     prestoMRN,
			PaymentRefNum: r.PathValue("paymentRefNum"),
		})
		if err != nil {
			writePrestoError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	}
}

func handleReverse(client *prestopay.Client, prestoMRN string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		paymentRefNum := r.PathValue("paymentRefNum")
		res, err := client.Payments.Reverse(r.Context(), prestopay.ReverseRequest{
			PrestoMRN:      prestoMRN,
			PaymentRefNum:  paymentRefNum,
			ReversalRefNum: fmt.Sprintf("rev-%d", time.Now().UnixNano()),
			Remark:         "requested via demo",
		})
		if err != nil {
			writePrestoError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	}
}

func handleRefund(client *prestopay.Client, prestoMRN string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		paymentRefNum := r.PathValue("paymentRefNum")
		res, err := client.Payments.Refund(r.Context(), prestopay.RefundRequest{
			PrestoMRN:     prestoMRN,
			PaymentRefNum: paymentRefNum,
			RefundRefNum:  fmt.Sprintf("rfnd-%d", time.Now().UnixNano()),
			Remark:        "requested via demo",
		})
		if err != nil {
			writePrestoError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	}
}

// handleNotify dedupes on EventRefNum before doing any fulfilment work, since
// Presto redelivers an undelivered webhook up to five times: acking an
// already-seen delivery with AckOK (rather than refulfilling) is what keeps
// that safe.
func handleNotify(verifier *prestopay.WebhookVerifier, deliveries *deliveryLog) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		event, err := verifier.VerifyRequest(r)
		if err != nil {
			log.Printf("webhook verification failed: %v", err)
			prestopay.WriteAck(w, prestopay.AckForError(err))
			return
		}
		if !deliveries.firstDelivery(event.EventRefNum) {
			log.Printf("duplicate webhook delivery for eventRefNum=%s; acking without refulfilling", event.EventRefNum)
			prestopay.WriteAck(w, prestopay.AckOK)
			return
		}
		log.Printf("webhook: prestoMrn=%s paymentRefNum=%s eventCode=%s paymentStatus=%s",
			event.PrestoMRN, event.PaymentRefNum, event.EventCode, event.PaymentStatus)
		prestopay.WriteAck(w, prestopay.AckOK)
	}
}

// writePrestoError reports a prestopay.Error's idempotency fields alongside
// the message, since that's the information a caller needs to decide
// whether it's safe to retry.
func writePrestoError(w http.ResponseWriter, err error) {
	status := http.StatusBadGateway
	body := map[string]any{"error": err.Error()}

	var ce *prestopay.ConfigError
	if errors.As(err, &ce) {
		status = http.StatusBadRequest
	}

	var pe prestopay.Error
	if errors.As(err, &pe) {
		body["mayHaveTakenEffect"] = pe.MayHaveTakenEffect()
		if key, ok := pe.ReconcileBy(); ok {
			body["reconcileBy"] = key
		}
	}
	writeJSON(w, status, body)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// deliveryLog is the demo's dedupe store. A real merchant would use its
// database's uniqueness constraints instead of an in-memory map.
type deliveryLog struct {
	mu   sync.Mutex
	seen map[string]bool
}

func newDeliveryLog() *deliveryLog { return &deliveryLog{seen: make(map[string]bool)} }

func (d *deliveryLog) firstDelivery(eventRefNum string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.seen[eventRefNum] {
		return false
	}
	d.seen[eventRefNum] = true
	return true
}

// loadDotEnv sets variables from a .env file without overriding anything
// already present in the real environment. It is a few lines rather than a
// dependency, since the SDK's own zero-dependency stance is worth keeping
// even in its examples.
func loadDotEnv(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		if _, alreadySet := os.LookupEnv(key); !alreadySet {
			os.Setenv(key, value)
		}
	}
}
