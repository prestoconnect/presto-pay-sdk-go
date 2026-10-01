// Command net-http is a runnable demo of the Go Presto Pay SDK against
// Presto's real staging gateway, using only net/http and the SDK itself —
// no third-party Go dependencies (the checkout page loads Tailwind CSS and
// Font Awesome from a CDN, but that's a browser asset, not a module import).
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
	"strings"
	"sync"
	"time"

	"github.com/prestoconnect/presto-pay-sdk-go/prestopay"
)

//go:embed templates/*.html
var templateFS embed.FS

var templates = template.Must(template.ParseFS(templateFS, "templates/*.html"))

type paymentMethodOption struct {
	Code string
	Name string
	Icon string
}

var paymentMethodChoices = []paymentMethodOption{
	{Code: prestopay.PaymentMethodCard, Name: "Credit / debit card", Icon: "fa-credit-card"},
	{Code: prestopay.PaymentMethodTouchNGoEWallet, Name: "Touch 'n Go eWallet", Icon: "fa-wallet"},
	{Code: prestopay.PaymentMethodGrabPay, Name: "GrabPay", Icon: "fa-wallet"},
	{Code: prestopay.PaymentMethodMaybank, Name: "Maybank FPX", Icon: "fa-building-columns"},
	{Code: prestopay.PaymentMethodCimb, Name: "CIMB Clicks", Icon: "fa-building-columns"},
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
		port = "8080"
	}
	baseURL := os.Getenv("PUBLIC_URL")
	if baseURL == "" {
		baseURL = "http://localhost:" + port
		log.Printf("WARNING: PUBLIC_URL is not set; using %s for redirects and notifyUrl. Presto cannot "+
			"deliver webhooks to localhost. Set PUBLIC_URL after exposing this port publicly to enable "+
			"webhook delivery.", baseURL)
	}

	st := newStore()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /", handleHome(st))
	mux.HandleFunc("POST /checkout", handleCheckout(client, prestoMRN, baseURL))
	mux.HandleFunc("GET /return/{txnRefNum}", handleReturn(client, prestoMRN))
	mux.HandleFunc("GET /payments/{paymentRefNum}", handleQuery(client, prestoMRN))
	mux.HandleFunc("POST /payments/{paymentRefNum}/reverse", handleReverse(client, prestoMRN))
	mux.HandleFunc("POST /payments/{paymentRefNum}/refund", handleRefund(client, prestoMRN))
	mux.HandleFunc("POST /presto/notify", handleNotify(client, verifier, st))

	log.Printf("Presto Pay SDK demo listening on http://localhost:%s", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}

func handleHome(st *store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data := struct {
			PaymentMethods []paymentMethodOption
			Events         []recentEvent
		}{
			PaymentMethods: paymentMethodChoices,
			Events:         st.recentEvents(),
		}
		if err := templates.ExecuteTemplate(w, "index.html", data); err != nil {
			log.Printf("rendering index.html: %v", err)
		}
	}
}

// handleCheckout always speaks JSON: the checkout page's own script submits
// it via fetch and does the redirect to PaymentURL itself, and curl gets the
// same response.
func handleCheckout(client *prestopay.Client, prestoMRN, baseURL string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			TxnRefNum     string `json:"txnRefNum"`
			Amount        int64  `json:"amount"`
			CurrencyCode  string `json:"currencyCode"`
			PaymentMethod string `json:"paymentMethod"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request body"})
			return
		}
		if input.TxnRefNum == "" {
			input.TxnRefNum = fmt.Sprintf("order-%d", time.Now().UnixNano())
		}
		if input.CurrencyCode == "" {
			input.CurrencyCode = "MYR"
		}
		if input.Amount <= 0 {
			input.Amount = 1000
		}
		var methods []string
		if input.PaymentMethod != "" {
			methods = []string{input.PaymentMethod}
		}

		res, err := client.Payments.Init(r.Context(), prestopay.InitRequest{
			PrestoMRN:             prestoMRN,
			TxnType:               prestopay.TxnTypeWebPay,
			TxnRefNum:             input.TxnRefNum,
			DisplayDesc:           "Go SDK demo order " + input.TxnRefNum,
			Amount:                input.Amount,
			CurrencyCode:          input.CurrencyCode,
			NotifyURL:             baseURL + "/presto/notify",
			RedirectURL:           baseURL + "/return/" + input.TxnRefNum,
			AllowedPaymentMethods: methods,
		})
		if err != nil {
			writePrestoError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	}
}

// returnStatus maps a payment status to how the return page presents it.
type returnStatus struct {
	Heading   string
	IconClass string
	IconBg    string
}

func classifyPaymentStatus(status string) returnStatus {
	switch status {
	case prestopay.PaymentStatusAuthorised, prestopay.PaymentStatusRefunded:
		return returnStatus{"Payment Successful", "fa-solid fa-check", "bg-green-600"}
	case prestopay.PaymentStatusPendingAuthorise, prestopay.PaymentStatusPendingReverse, prestopay.PaymentStatusPendingRefund:
		return returnStatus{"Payment Pending", "fa-solid fa-clock", "bg-amber-500"}
	case prestopay.PaymentStatusFailed, prestopay.PaymentStatusCancelled, prestopay.PaymentStatusExpired:
		return returnStatus{"Payment Failed", "fa-solid fa-xmark", "bg-red-600"}
	default:
		return returnStatus{"Payment " + status, "fa-solid fa-circle-info", "bg-slate-500"}
	}
}

func handleReturn(client *prestopay.Client, prestoMRN string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		txnRefNum := r.PathValue("txnRefNum")
		res, err := client.Payments.Query(r.Context(), prestopay.QueryRequest{
			PrestoMRN: prestoMRN,
			TxnRefNum: txnRefNum,
		})

		data := struct {
			Error           string
			TxnRefNum       string
			PaymentRefNum   string
			PaymentStatus   string
			FormattedAmount string
			CurrencyCode    string
			Heading         string
			IconClass       string
			IconBg          string
		}{TxnRefNum: txnRefNum}

		if err != nil {
			data.Error = err.Error()
		} else {
			status := classifyPaymentStatus(res.PaymentStatus)
			data.PaymentRefNum = res.PaymentRefNum
			data.PaymentStatus = res.PaymentStatus
			data.FormattedAmount = fmt.Sprintf("%.2f", float64(res.Amount)/100)
			data.CurrencyCode = res.CurrencyCode
			data.Heading = status.Heading
			data.IconClass = status.IconClass
			data.IconBg = status.IconBg
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
func handleNotify(client *prestopay.Client, verifier *prestopay.WebhookVerifier, st *store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		event, err := verifier.VerifyRequest(r)
		if err != nil {
			log.Printf("webhook verification failed: %v", err)
			var sigErr *prestopay.SignatureError
			if errors.As(err, &sigErr) {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			prestopay.WriteAck(w, prestopay.AckForError(err))
			return
		}
		// A webhook says what happened, not the payment's resulting status, so
		// ask Presto. The event is only marked as seen after this succeeds: if
		// the query fails, AckForError asks for a resend, and that redelivery
		// must not be mistaken for a duplicate.
		payment, err := client.Payments.Query(r.Context(), prestopay.QueryRequest{
			PrestoMRN:     event.PrestoMRN,
			PaymentRefNum: event.PaymentRefNum,
		})
		if err != nil {
			log.Printf("webhook eventRefNum=%s: query failed, asking Presto to resend: %v", event.EventRefNum, err)
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
			PaymentStatus: payment.PaymentStatus,
		})
		log.Printf("webhook: prestoMrn=%s paymentRefNum=%s eventCode=%s success=%t queried paymentStatus=%s",
			event.PrestoMRN, event.PaymentRefNum, event.EventCode, event.Success, payment.PaymentStatus)
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

// recentEvent is a webhook delivery shown on the checkout page.
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
