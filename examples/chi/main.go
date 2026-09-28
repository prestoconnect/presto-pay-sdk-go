// Command chi is a runnable demo of the Go Presto Pay SDK against Presto's
// real staging gateway, routed with chi instead of the stdlib mux — the
// handlers themselves are unchanged from the net-http example, since the
// SDK has no opinion about which router calls it.
package main

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/prestoconnect/presto-pay-sdk-go/prestopay"
)

//go:embed templates/*.html
var templateFS embed.FS

var templates = template.Must(template.ParseFS(templateFS, "templates/*.html"))

var paymentMethodChoices = []string{
	prestopay.PaymentMethodWallet,
	prestopay.PaymentMethodCard,
	prestopay.PaymentMethodTouchNGoEWallet,
	prestopay.PaymentMethodGrabPay,
	prestopay.PaymentMethodBoost,
}

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
		port = "8081"
	}
	baseURL := os.Getenv("PUBLIC_URL")
	if baseURL == "" {
		baseURL = "http://localhost:" + port
		log.Printf("WARNING: PUBLIC_URL is not set; using %s for redirects and notifyUrl. Presto cannot "+
			"deliver webhooks to localhost. Set PUBLIC_URL after exposing this port publicly to enable "+
			"webhook delivery.", baseURL)
	}

	st := newStore()

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get("/", handleHome(st))
	r.Post("/checkout", handleCheckout(client, prestoMRN, baseURL))
	r.Get("/return/{txnRefNum}", handleReturn(client, prestoMRN))
	r.Get("/payments/{paymentRefNum}", handleQuery(client, prestoMRN))
	r.Post("/payments/{paymentRefNum}/reverse", handleReverse(client, prestoMRN))
	r.Post("/payments/{paymentRefNum}/refund", handleRefund(client, prestoMRN))
	r.Post("/presto/notify", handleNotify(verifier, st))

	log.Printf("Presto Pay SDK chi demo listening on http://localhost:%s", port)
	log.Fatal(http.ListenAndServe(":"+port, r))
}

func handleHome(st *store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data := struct {
			ShowMethods    bool
			PaymentMethods []string
			Events         []recentEvent
		}{
			ShowMethods:    r.URL.Query().Get("showMethods") == "1",
			PaymentMethods: paymentMethodChoices,
			Events:         st.recentEvents(),
		}
		if err := templates.ExecuteTemplate(w, "index.html", data); err != nil {
			log.Printf("rendering index.html: %v", err)
		}
	}
}

// handleCheckout serves both the browser form (redirects to the hosted
// payment page on success) and a JSON API for curl, distinguished by
// Content-Type.
func handleCheckout(client *prestopay.Client, prestoMRN, baseURL string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		isJSON := strings.Contains(r.Header.Get("Content-Type"), "application/json")

		var (
			txnRefNum    string
			amount       int64
			currencyCode string
			methods      []string
		)
		if isJSON {
			var input struct {
				TxnRefNum    string `json:"txnRefNum"`
				Amount       int64  `json:"amount"`
				CurrencyCode string `json:"currencyCode"`
			}
			_ = json.NewDecoder(r.Body).Decode(&input)
			txnRefNum, amount, currencyCode = input.TxnRefNum, input.Amount, input.CurrencyCode
		} else {
			_ = r.ParseForm()
			amount, _ = strconv.ParseInt(r.FormValue("amount"), 10, 64)
			currencyCode = r.FormValue("currencyCode")
			if method := r.FormValue("paymentMethod"); method != "" {
				methods = []string{method}
			}
		}
		if txnRefNum == "" {
			txnRefNum = fmt.Sprintf("order-%d", time.Now().UnixNano())
		}
		if currencyCode == "" {
			currencyCode = "MYR"
		}
		if amount == 0 {
			amount = 1000
		}

		res, err := client.Payments.Init(r.Context(), prestopay.InitRequest{
			PrestoMRN:             prestoMRN,
			TxnType:               prestopay.TxnTypeWebPay,
			TxnRefNum:             txnRefNum,
			DisplayDesc:           "Go SDK chi demo order " + txnRefNum,
			Amount:                amount,
			CurrencyCode:          currencyCode,
			NotifyURL:             baseURL + "/presto/notify",
			RedirectURL:           baseURL + "/return/" + txnRefNum,
			AllowedPaymentMethods: methods,
		})
		if err != nil {
			if isJSON {
				writePrestoError(w, err)
				return
			}
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}

		if isJSON {
			writeJSON(w, http.StatusOK, res)
			return
		}
		http.Redirect(w, r, res.PaymentURL, http.StatusSeeOther)
	}
}

func handleReturn(client *prestopay.Client, prestoMRN string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		txnRefNum := chi.URLParam(r, "txnRefNum")
		res, err := client.Payments.Query(r.Context(), prestopay.QueryRequest{
			PrestoMRN: prestoMRN,
			TxnRefNum: txnRefNum,
		})

		data := struct {
			Error         string
			TxnRefNum     string
			PaymentRefNum string
			PaymentStatus string
			Amount        int64
			CurrencyCode  string
		}{TxnRefNum: txnRefNum}
		if err != nil {
			data.Error = err.Error()
		} else {
			data.PaymentRefNum = res.PaymentRefNum
			data.PaymentStatus = res.PaymentStatus
			data.Amount = res.Amount
			data.CurrencyCode = res.CurrencyCode
		}
		if err := templates.ExecuteTemplate(w, "return.html", data); err != nil {
			log.Printf("rendering return.html: %v", err)
		}
	}
}

func handleQuery(client *prestopay.Client, prestoMRN string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		res, err := client.Payments.Query(r.Context(), prestopay.QueryRequest{
			PrestoMRN:     prestoMRN,
			PaymentRefNum: chi.URLParam(r, "paymentRefNum"),
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
		paymentRefNum := chi.URLParam(r, "paymentRefNum")
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
		paymentRefNum := chi.URLParam(r, "paymentRefNum")
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
func handleNotify(verifier *prestopay.WebhookVerifier, st *store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		event, err := verifier.VerifyRequest(r)
		if err != nil {
			log.Printf("webhook verification failed: %v", err)
			prestopay.WriteAck(w, prestopay.AckForError(err))
			return
		}
		if !st.firstDelivery(event.EventRefNum) {
			log.Printf("duplicate webhook delivery for eventRefNum=%s; acking without refulfilling", event.EventRefNum)
			prestopay.WriteAck(w, prestopay.AckOK)
			return
		}
		st.recordEvent(recentEvent{
			ReceivedAt:    time.Now().Format(time.RFC3339),
			EventCode:     event.EventCode,
			PaymentRefNum: event.PaymentRefNum,
			PaymentStatus: event.PaymentStatus,
		})
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

// recentEvent is a webhook delivery shown on the checkout and return pages.
type recentEvent struct {
	ReceivedAt    string
	EventCode     string
	PaymentRefNum string
	PaymentStatus string
}

const maxRecentEvents = 20

// store is the demo's dedupe and recent-activity state. A real merchant
// would use its database's uniqueness constraints instead of an in-memory
// map, and would not need the recent-events list at all.
type store struct {
	mu     sync.Mutex
	seen   map[string]bool
	events []recentEvent
}

func newStore() *store { return &store{seen: make(map[string]bool)} }

func (s *store) firstDelivery(eventRefNum string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.seen[eventRefNum] {
		return false
	}
	s.seen[eventRefNum] = true
	return true
}

func (s *store) recordEvent(e recentEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append([]recentEvent{e}, s.events...)
	if len(s.events) > maxRecentEvents {
		s.events = s.events[:maxRecentEvents]
	}
}

func (s *store) recentEvents() []recentEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]recentEvent, len(s.events))
	copy(out, s.events)
	return out
}

// loadDotEnv sets variables from a .env file without overriding anything
// already present in the real environment.
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
