package prestopay

import (
	"context"
	"crypto/rsa"
	"net/http"
	"time"

	"github.com/prestoconnect/presto-pay-sdk-go/prestopay/internal/keys"
)

// Environment selects a preset base URL.
type Environment int

const (
	Staging Environment = iota
	Production
)

const (
	stagingBaseURL    = "https://presto-stg-ext.enovax.com"
	productionBaseURL = "https://pay-ext.prestouniverse.com"

	defaultDeadline = 30 * time.Second
)

// RetryReads configures retries for Query, the one operation safe to resend.
// Init, Reverse, and Refund are never retried by this policy — they are
// retried only when an httptrace-proven RequestNotSent shows nothing reached
// the gateway, which needs no configuration.
type RetryReads struct {
	MaxRetries     int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
}

// Config configures a Client.
type Config struct {
	// Environment picks a preset base URL; BaseURL, if set, overrides it.
	Environment Environment
	BaseURL     string

	MerchantID       string
	PrivateKeyPEM    []byte
	PrestoPublicKeys [][]byte

	// Deadline is the budget for a whole call, retries and backoff
	// included; each attempt gets a child context with whatever remains.
	// Zero means the default of 30 seconds.
	Deadline time.Duration

	RetryReads RetryReads

	// Strict rejects a response that violates a rule the gateway has
	// confirmed but this SDK otherwise tolerates. Use it in staging to catch
	// contract drift; leave it false in production, where rejecting an
	// authentic response after the operation already took effect helps
	// nobody.
	Strict bool

	// ShowErrorBodies includes RawBody and Canonical in Error() strings.
	// The zero value (false) redacts them, since they can carry cardBin,
	// cardSummary, receiptEmail, or receiptName and errors routinely end up
	// in logs; the unredacted values are always on the error's own fields
	// regardless of this setting.
	ShowErrorBodies bool

	// HTTPClient is used as the base client if set; CheckRedirect is
	// overridden regardless.
	HTTPClient *http.Client

	// Now overrides the clock used for request timestamps; nil means
	// time.Now.
	Now func() time.Time
}

// Client is the entry point for calling the Presto Pay gateway.
// A *Client is safe for concurrent use and is meant to be built once and
// shared, like *sql.DB.
type Client struct {
	config Config

	baseURL string

	privateKey       *rsa.PrivateKey
	prestoPublicKeys []*rsa.PublicKey

	httpClient *http.Client

	deadline        time.Duration
	retryReads      RetryReads
	strict          bool
	showErrorBodies bool
	now             func() time.Time
}

// New builds a Client from cfg, loading and validating the merchant private
// key and Presto public keys eagerly: a bad key is a *ConfigError raised at
// construction, not a mysterious failure on the first real call.
func New(cfg Config) (*Client, error) {
	c := &Client{config: cfg}

	switch {
	case cfg.BaseURL != "":
		c.baseURL = cfg.BaseURL
	case cfg.Environment == Production:
		c.baseURL = productionBaseURL
	default:
		c.baseURL = stagingBaseURL
	}

	if cfg.MerchantID == "" {
		return nil, &ConfigError{Op: OpConfig, Field: "MerchantID"}
	}

	privateKey, err := keys.LoadPrivateKey(cfg.PrivateKeyPEM)
	if err != nil {
		return nil, &ConfigError{Op: OpConfig, Field: "PrivateKeyPEM", Err: err}
	}
	c.privateKey = privateKey

	if len(cfg.PrestoPublicKeys) == 0 {
		return nil, &ConfigError{Op: OpConfig, Field: "PrestoPublicKeys"}
	}
	for _, raw := range cfg.PrestoPublicKeys {
		pub, err := keys.LoadPrestoPublicKey(raw)
		if err != nil {
			return nil, &ConfigError{Op: OpConfig, Field: "PrestoPublicKeys", Err: err}
		}
		c.prestoPublicKeys = append(c.prestoPublicKeys, pub)
	}

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{}
	} else {
		clone := *httpClient
		httpClient = &clone
	}
	// A 3xx with a signed payment body attached is an HTTP error, never
	// something to follow transparently.
	httpClient.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	c.httpClient = httpClient

	c.deadline = cfg.Deadline
	if c.deadline <= 0 {
		c.deadline = defaultDeadline
	}
	c.retryReads = cfg.RetryReads
	c.strict = cfg.Strict
	c.showErrorBodies = cfg.ShowErrorBodies

	c.now = cfg.Now
	if c.now == nil {
		c.now = time.Now
	}

	return c, nil
}

// withDeadline bounds ctx to the whole-call budget. The returned cancel func
// must be called once the call completes.
func (c *Client) withDeadline(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, c.deadline)
}
