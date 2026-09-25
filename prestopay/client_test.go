package prestopay

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"net/http"
	"testing"
	"time"
)

func testMaterial(t *testing.T) (privatePEM, certPEM []byte) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("MarshalPKCS8PrivateKey: %v", err)
	}
	privatePEM = pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})

	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Hour),
	}
	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("CreateCertificate: %v", err)
	}
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	return privatePEM, certPEM
}

func validConfig(t *testing.T) Config {
	priv, cert := testMaterial(t)
	return Config{
		MerchantID:       "MID1",
		PrivateKeyPEM:    priv,
		PrestoPublicKeys: [][]byte{cert},
	}
}

func TestNew_ValidConfig(t *testing.T) {
	c, err := New(validConfig(t))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if c.baseURL != stagingBaseURL {
		t.Fatalf("baseURL = %q, want staging default", c.baseURL)
	}
	if c.deadline != defaultDeadline {
		t.Fatalf("deadline = %v, want default %v", c.deadline, defaultDeadline)
	}
	if c.privateKey == nil || len(c.prestoPublicKeys) != 1 {
		t.Fatal("keys were not loaded")
	}
}

func TestNew_MissingMerchantID(t *testing.T) {
	cfg := validConfig(t)
	cfg.MerchantID = ""
	_, err := New(cfg)
	var ce *ConfigError
	if !errors.As(err, &ce) || ce.Field != "MerchantID" {
		t.Fatalf("New() error = %v, want *ConfigError{Field: MerchantID}", err)
	}
}

func TestNew_BadPrivateKey(t *testing.T) {
	cfg := validConfig(t)
	cfg.PrivateKeyPEM = []byte("not a key")
	_, err := New(cfg)
	var ce *ConfigError
	if !errors.As(err, &ce) || ce.Field != "PrivateKeyPEM" {
		t.Fatalf("New() error = %v, want *ConfigError{Field: PrivateKeyPEM}", err)
	}
	if !errors.Is(err, ErrNoPEMBlock) {
		t.Fatalf("New() error should wrap ErrNoPEMBlock: %v", err)
	}
}

func TestNew_NoPublicKeys(t *testing.T) {
	cfg := validConfig(t)
	cfg.PrestoPublicKeys = nil
	_, err := New(cfg)
	var ce *ConfigError
	if !errors.As(err, &ce) || ce.Field != "PrestoPublicKeys" {
		t.Fatalf("New() error = %v, want *ConfigError{Field: PrestoPublicKeys}", err)
	}
}

func TestNew_BadPublicKey(t *testing.T) {
	cfg := validConfig(t)
	cfg.PrestoPublicKeys = [][]byte{[]byte("garbage")}
	_, err := New(cfg)
	var ce *ConfigError
	if !errors.As(err, &ce) || ce.Field != "PrestoPublicKeys" {
		t.Fatalf("New() error = %v, want *ConfigError{Field: PrestoPublicKeys}", err)
	}
}

func TestNew_ProductionBaseURL(t *testing.T) {
	cfg := validConfig(t)
	cfg.Environment = Production
	c, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if c.baseURL != productionBaseURL {
		t.Fatalf("baseURL = %q, want production", c.baseURL)
	}
}

func TestNew_ExplicitBaseURLOverridesEnvironment(t *testing.T) {
	cfg := validConfig(t)
	cfg.Environment = Production
	cfg.BaseURL = "https://custom.example"
	c, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if c.baseURL != "https://custom.example" {
		t.Fatalf("baseURL = %q, want the explicit override", c.baseURL)
	}
}

func TestNew_ForcesCheckRedirect(t *testing.T) {
	cfg := validConfig(t)
	cfg.HTTPClient = &http.Client{}
	c, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if c.httpClient.CheckRedirect == nil {
		t.Fatal("CheckRedirect was not set")
	}
	if got := c.httpClient.CheckRedirect(nil, nil); got != http.ErrUseLastResponse {
		t.Fatalf("CheckRedirect() = %v, want http.ErrUseLastResponse", got)
	}
	if cfg.HTTPClient.CheckRedirect != nil {
		t.Fatal("New must not mutate the caller's *http.Client in place")
	}
}

func TestNew_CustomDeadlineAndNow(t *testing.T) {
	cfg := validConfig(t)
	cfg.Deadline = 5 * time.Second
	fixed := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	cfg.Now = func() time.Time { return fixed }
	c, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if c.deadline != 5*time.Second {
		t.Fatalf("deadline = %v, want 5s", c.deadline)
	}
	if !c.now().Equal(fixed) {
		t.Fatalf("now() = %v, want %v", c.now(), fixed)
	}
}
