// Package keys loads the merchant private key and Presto public keys from
// PEM or DER input.
package keys

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
)

// Sentinel errors for the specific ways a key input can be wrong. Each is
// wrapped with fmt.Errorf so errors.Is still matches, while Error() carries
// the fix — a merchant reports "the key won't load" long before they read
// documentation, so the fix has to live in the message.
var (
	ErrKeyNotRSA        = errors.New("prestopay: key is not RSA")
	ErrKeyTooSmall      = errors.New("prestopay: RSA key is smaller than 2048 bits")
	ErrEncryptedKey     = errors.New("prestopay: private key is encrypted")
	ErrPKCS1Key         = errors.New("prestopay: private key is PKCS#1, not PKCS#8")
	ErrCertificateAsKey = errors.New("prestopay: input is a certificate, not a private key")
	ErrNoPEMBlock       = errors.New("prestopay: no PEM block found")
	ErrUnknownPEMType   = errors.New("prestopay: unrecognized PEM block type")
)

const minRSABits = 2048

// LoadPrivateKey parses a PKCS#8 PEM-encoded RSA private key, as delivered
// after converting the onboarding PKCS#12 keystore (go-plan.md §8).
func LoadPrivateKey(pemBytes []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		// The most likely cause of "not PEM at all" is the onboarding
		// PKCS#12 keystore passed in unconverted, so the fix goes in the
		// message rather than making the caller go find it.
		return nil, fmt.Errorf("%w; if this is the onboarding .p12 keystore, convert it first: "+
			"openssl pkcs12 -in partner.p12 -nocerts -nodes -out partner-key.pem", ErrNoPEMBlock)
	}

	switch block.Type {
	case "PRIVATE KEY":
		// fall through
	case "ENCRYPTED PRIVATE KEY":
		return nil, fmt.Errorf("%w; decrypt it to a plain PKCS#8 PEM before passing it in", ErrEncryptedKey)
	case "RSA PRIVATE KEY":
		return nil, fmt.Errorf("%w; convert it: openssl pkcs8 -topk8 -nocrypt -in key.pem -out key-pkcs8.pem", ErrPKCS1Key)
	case "CERTIFICATE":
		return nil, fmt.Errorf("%w", ErrCertificateAsKey)
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnknownPEMType, block.Type)
	}

	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("prestopay: parsing PKCS#8 private key: %w", err)
	}

	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("%w: got %T", ErrKeyNotRSA, parsed)
	}
	if key.N.BitLen() < minRSABits {
		return nil, fmt.Errorf("%w: got %d bits", ErrKeyTooSmall, key.N.BitLen())
	}
	return key, nil
}

// LoadPrestoPublicKey parses Presto's public key from an X.509 certificate
// (PEM or DER) or a PKIX/SPKI PEM block.
func LoadPrestoPublicKey(data []byte) (*rsa.PublicKey, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		// No PEM armor: try it as a DER certificate directly.
		cert, err := x509.ParseCertificate(data)
		if err != nil {
			return nil, fmt.Errorf("prestopay: not a PEM block and not a DER certificate: %w", err)
		}
		return publicRSAKey(cert.PublicKey)
	}

	switch block.Type {
	case "CERTIFICATE":
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("prestopay: parsing certificate: %w", err)
		}
		return publicRSAKey(cert.PublicKey)
	case "PUBLIC KEY":
		parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("prestopay: parsing PKIX public key: %w", err)
		}
		return publicRSAKey(parsed)
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnknownPEMType, block.Type)
	}
}

func publicRSAKey(pub any) (*rsa.PublicKey, error) {
	key, ok := pub.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("%w: got %T", ErrKeyNotRSA, pub)
	}
	if key.N.BitLen() < minRSABits {
		return nil, fmt.Errorf("%w: got %d bits", ErrKeyTooSmall, key.N.BitLen())
	}
	return key, nil
}

