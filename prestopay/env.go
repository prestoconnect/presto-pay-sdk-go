package prestopay

import (
	"fmt"
	"os"
)

// ConfigFromEnv builds a Config from the PRESTOPAY_* variables, read through
// getenv rather than os.Getenv directly, so tests and env-style libraries
// work without this package reaching for the process environment.
func ConfigFromEnv(getenv func(string) string) (Config, error) {
	var cfg Config

	if baseURL := getenv("PRESTOPAY_BASE_URL"); baseURL != "" {
		cfg.BaseURL = baseURL
	} else {
		switch env := getenv("PRESTOPAY_ENV"); env {
		case "staging":
			cfg.Environment = Staging
		case "production":
			cfg.Environment = Production
		default:
			return Config{}, &ConfigError{Op: OpConfig, Field: "PRESTOPAY_ENV",
				Err: fmt.Errorf(`must be "staging" or "production" (or set PRESTOPAY_BASE_URL instead), got %q`, env)}
		}
	}

	mid := getenv("PRESTOPAY_MID")
	if mid == "" {
		return Config{}, &ConfigError{Op: OpConfig, Field: "PRESTOPAY_MID", Err: errMissingEnvVar}
	}
	cfg.MerchantID = mid

	privateKey, err := readKeyFromEnv(getenv, "PRESTOPAY_PRIVATE_KEY", "PRESTOPAY_PRIVATE_KEY_FILE")
	if err != nil {
		return Config{}, &ConfigError{Op: OpConfig, Field: "PRESTOPAY_PRIVATE_KEY", Err: err}
	}
	cfg.PrivateKeyPEM = privateKey

	publicKey, err := readKeyFromEnv(getenv, "PRESTOPAY_PUBLIC_KEY", "PRESTOPAY_PUBLIC_KEY_FILE")
	if err != nil {
		return Config{}, &ConfigError{Op: OpConfig, Field: "PRESTOPAY_PUBLIC_KEY", Err: err}
	}
	cfg.PrestoPublicKeys = [][]byte{publicKey}

	return cfg, nil
}

var errMissingEnvVar = fmt.Errorf("required environment variable is missing")

// readKeyFromEnv reads a key from envVar directly, or from the file named by
// fileVar if envVar is unset.
func readKeyFromEnv(getenv func(string) string, envVar, fileVar string) ([]byte, error) {
	if v := getenv(envVar); v != "" {
		return []byte(v), nil
	}
	path := getenv(fileVar)
	if path == "" {
		return nil, fmt.Errorf("%s or %s is required", envVar, fileVar)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s (%s): %w", fileVar, path, err)
	}
	return data, nil
}
