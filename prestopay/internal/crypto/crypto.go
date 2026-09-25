// Package crypto signs and verifies canonical strings with RSASSA-PKCS1-v1_5
// over SHA-256.
package crypto

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"errors"
)

// ErrInvalidSignature covers both a signature that is not valid Base64 and
// one that is well-formed but does not verify: the gateway's own error codes
// do not distinguish the two (go-plan.md §3.6, codes 1006/1007), so neither
// does this package.
var ErrInvalidSignature = errors.New("prestopay: signature invalid")

// Sign returns the standard-Base64 signature over the UTF-8 bytes of
// canonical, using the merchant's RSA private key.
func Sign(key *rsa.PrivateKey, canonical string) (string, error) {
	sum := sha256.Sum256([]byte(canonical))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(sig), nil
}

// Verify checks sigB64 against the UTF-8 bytes of canonical, using Presto's
// RSA public key. Non-canonical Base64 padding is rejected: a signature that
// decodes only under a lenient decoder is not the byte string that was
// produced by an RSA sign operation.
func Verify(key *rsa.PublicKey, canonical string, sigB64 string) error {
	sig, err := base64.StdEncoding.Strict().DecodeString(sigB64)
	if err != nil {
		return ErrInvalidSignature
	}
	sum := sha256.Sum256([]byte(canonical))
	if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, sum[:], sig); err != nil {
		return ErrInvalidSignature
	}
	return nil
}
