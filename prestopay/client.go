package prestopay

// Config configures a Client. See go-plan.md §4 for the full field list;
// this is scaffolding pending milestone 2 onward.
type Config struct {
	Environment Environment
	BaseURL     string

	MerchantID       string
	PrivateKeyPEM    []byte
	PrestoPublicKeys [][]byte
}

// Environment selects a preset base URL.
type Environment int

const (
	Staging Environment = iota
	Production
)

// Client is the entry point for calling the Presto Pay gateway.
// A *Client is safe for concurrent use and is meant to be built once and shared.
type Client struct {
	config Config
}

// New builds a Client from cfg.
func New(cfg Config) (*Client, error) {
	return &Client{config: cfg}, nil
}
