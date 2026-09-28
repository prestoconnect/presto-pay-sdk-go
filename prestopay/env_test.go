package prestopay

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func fakeGetenv(vars map[string]string) func(string) string {
	return func(key string) string { return vars[key] }
}

func TestConfigFromEnv_InlineValues(t *testing.T) {
	cfg, err := ConfigFromEnv(fakeGetenv(map[string]string{
		"PRESTOPAY_ENV":         "staging",
		"PRESTOPAY_MID":         "MID1",
		"PRESTOPAY_PRIVATE_KEY": "priv-pem",
		"PRESTOPAY_PUBLIC_KEY":  "pub-pem",
	}))
	if err != nil {
		t.Fatalf("ConfigFromEnv: %v", err)
	}
	if cfg.Environment != Staging || cfg.MerchantID != "MID1" {
		t.Fatalf("cfg = %+v", cfg)
	}
	if string(cfg.PrivateKeyPEM) != "priv-pem" || string(cfg.PrestoPublicKeys[0]) != "pub-pem" {
		t.Fatalf("cfg = %+v", cfg)
	}
}

func TestConfigFromEnv_FileVariants(t *testing.T) {
	dir := t.TempDir()
	privPath := filepath.Join(dir, "priv.pem")
	pubPath := filepath.Join(dir, "pub.pem")
	if err := writeFile(privPath, "priv-from-file"); err != nil {
		t.Fatal(err)
	}
	if err := writeFile(pubPath, "pub-from-file"); err != nil {
		t.Fatal(err)
	}

	cfg, err := ConfigFromEnv(fakeGetenv(map[string]string{
		"PRESTOPAY_ENV":              "production",
		"PRESTOPAY_MID":              "MID1",
		"PRESTOPAY_PRIVATE_KEY_FILE": privPath,
		"PRESTOPAY_PUBLIC_KEY_FILE":  pubPath,
	}))
	if err != nil {
		t.Fatalf("ConfigFromEnv: %v", err)
	}
	if string(cfg.PrivateKeyPEM) != "priv-from-file" || string(cfg.PrestoPublicKeys[0]) != "pub-from-file" {
		t.Fatalf("cfg = %+v", cfg)
	}
}

func TestConfigFromEnv_BaseURLOverridesEnv(t *testing.T) {
	cfg, err := ConfigFromEnv(fakeGetenv(map[string]string{
		"PRESTOPAY_BASE_URL":    "https://custom.example",
		"PRESTOPAY_MID":         "MID1",
		"PRESTOPAY_PRIVATE_KEY": "priv-pem",
		"PRESTOPAY_PUBLIC_KEY":  "pub-pem",
	}))
	if err != nil {
		t.Fatalf("ConfigFromEnv: %v", err)
	}
	if cfg.BaseURL != "https://custom.example" {
		t.Fatalf("BaseURL = %q, want the explicit override", cfg.BaseURL)
	}
}

func TestConfigFromEnv_MissingEnvOrBaseURL(t *testing.T) {
	_, err := ConfigFromEnv(fakeGetenv(map[string]string{"PRESTOPAY_MID": "MID1"}))
	var ce *ConfigError
	if !errors.As(err, &ce) || ce.Field != "PRESTOPAY_ENV" {
		t.Fatalf("ConfigFromEnv() error = %v, want *ConfigError{Field: PRESTOPAY_ENV}", err)
	}
}

func TestConfigFromEnv_MissingMID(t *testing.T) {
	_, err := ConfigFromEnv(fakeGetenv(map[string]string{"PRESTOPAY_ENV": "staging"}))
	var ce *ConfigError
	if !errors.As(err, &ce) || ce.Field != "PRESTOPAY_MID" {
		t.Fatalf("ConfigFromEnv() error = %v, want *ConfigError{Field: PRESTOPAY_MID}", err)
	}
}

func TestConfigFromEnv_MissingPrivateKey(t *testing.T) {
	_, err := ConfigFromEnv(fakeGetenv(map[string]string{
		"PRESTOPAY_ENV": "staging",
		"PRESTOPAY_MID": "MID1",
	}))
	var ce *ConfigError
	if !errors.As(err, &ce) || ce.Field != "PRESTOPAY_PRIVATE_KEY" {
		t.Fatalf("ConfigFromEnv() error = %v, want *ConfigError{Field: PRESTOPAY_PRIVATE_KEY}", err)
	}
}

func TestConfigFromEnv_UnreadableKeyFile(t *testing.T) {
	_, err := ConfigFromEnv(fakeGetenv(map[string]string{
		"PRESTOPAY_ENV":              "staging",
		"PRESTOPAY_MID":              "MID1",
		"PRESTOPAY_PRIVATE_KEY_FILE": filepath.Join(t.TempDir(), "does-not-exist.pem"),
	}))
	var ce *ConfigError
	if !errors.As(err, &ce) || ce.Field != "PRESTOPAY_PRIVATE_KEY" {
		t.Fatalf("ConfigFromEnv() error = %v, want *ConfigError{Field: PRESTOPAY_PRIVATE_KEY}", err)
	}
}

func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o600)
}
