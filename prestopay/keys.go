package prestopay

import (
	"crypto/rsa"

	"github.com/prestoconnect/presto-pay-sdk-go/prestopay/internal/keys"
)

// Sentinel errors from key loading, re-exported so callers can use errors.Is
// without reaching into an internal package.
var (
	ErrKeyNotRSA        = keys.ErrKeyNotRSA
	ErrKeyTooSmall      = keys.ErrKeyTooSmall
	ErrEncryptedKey     = keys.ErrEncryptedKey
	ErrPKCS1Key         = keys.ErrPKCS1Key
	ErrCertificateAsKey = keys.ErrCertificateAsKey
	ErrNoPEMBlock       = keys.ErrNoPEMBlock
	ErrUnknownPEMType   = keys.ErrUnknownPEMType
)

// LoadPrivateKey parses a PKCS#8 PEM-encoded RSA private key of at least
// 2048 bits.
func LoadPrivateKey(pemBytes []byte) (*rsa.PrivateKey, error) {
	return keys.LoadPrivateKey(pemBytes)
}

// LoadPrestoPublicKey parses Presto's public key from an X.509 certificate
// (PEM or DER) or a PKIX/SPKI PEM block.
func LoadPrestoPublicKey(data []byte) (*rsa.PublicKey, error) {
	return keys.LoadPrestoPublicKey(data)
}
