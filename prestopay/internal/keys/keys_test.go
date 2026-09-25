package keys

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"testing"
	"time"
)

func genRSAKey(t *testing.T, bits int) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	return key
}

func pkcs8PEM(t *testing.T, key *rsa.PrivateKey) []byte {
	t.Helper()
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("MarshalPKCS8PrivateKey: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
}

func pkcs1PEM(t *testing.T, key *rsa.PrivateKey) []byte {
	t.Helper()
	return pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
}

func selfSignedCertPEM(t *testing.T, key *rsa.PrivateKey) []byte {
	t.Helper()
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("CreateCertificate: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

func spkiPEM(t *testing.T, pub *rsa.PublicKey) []byte {
	t.Helper()
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		t.Fatalf("MarshalPKIXPublicKey: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})
}

func TestLoadPrivateKey_PKCS8PEM(t *testing.T) {
	key := genRSAKey(t, 2048)
	loaded, err := LoadPrivateKey(pkcs8PEM(t, key))
	if err != nil {
		t.Fatalf("LoadPrivateKey: %v", err)
	}
	if loaded.N.Cmp(key.N) != 0 {
		t.Fatal("loaded key does not match original")
	}
}

func TestLoadPrivateKey_RejectsPKCS1(t *testing.T) {
	key := genRSAKey(t, 2048)
	_, err := LoadPrivateKey(pkcs1PEM(t, key))
	if !errors.Is(err, ErrPKCS1Key) {
		t.Fatalf("LoadPrivateKey() error = %v, want ErrPKCS1Key", err)
	}
}

func TestLoadPrivateKey_RejectsCertificate(t *testing.T) {
	key := genRSAKey(t, 2048)
	_, err := LoadPrivateKey(selfSignedCertPEM(t, key))
	if !errors.Is(err, ErrCertificateAsKey) {
		t.Fatalf("LoadPrivateKey() error = %v, want ErrCertificateAsKey", err)
	}
}

func TestLoadPrivateKey_RejectsTooSmall(t *testing.T) {
	key := genRSAKey(t, 1024)
	_, err := LoadPrivateKey(pkcs8PEM(t, key))
	if !errors.Is(err, ErrKeyTooSmall) {
		t.Fatalf("LoadPrivateKey() error = %v, want ErrKeyTooSmall", err)
	}
}

func TestLoadPrivateKey_NoPEMBlock(t *testing.T) {
	_, err := LoadPrivateKey([]byte("not pem at all"))
	if !errors.Is(err, ErrNoPEMBlock) {
		t.Fatalf("LoadPrivateKey() error = %v, want ErrNoPEMBlock", err)
	}
}

func TestLoadPrestoPublicKey_CertificatePEM(t *testing.T) {
	key := genRSAKey(t, 2048)
	pub, err := LoadPrestoPublicKey(selfSignedCertPEM(t, key))
	if err != nil {
		t.Fatalf("LoadPrestoPublicKey: %v", err)
	}
	if pub.N.Cmp(key.PublicKey.N) != 0 {
		t.Fatal("loaded public key does not match original")
	}
}

func TestLoadPrestoPublicKey_CertificateDER(t *testing.T) {
	key := genRSAKey(t, 2048)
	block, _ := pem.Decode(selfSignedCertPEM(t, key))
	pub, err := LoadPrestoPublicKey(block.Bytes)
	if err != nil {
		t.Fatalf("LoadPrestoPublicKey (DER): %v", err)
	}
	if pub.N.Cmp(key.PublicKey.N) != 0 {
		t.Fatal("loaded public key does not match original")
	}
}

func TestLoadPrestoPublicKey_SPKIPEM(t *testing.T) {
	key := genRSAKey(t, 2048)
	pub, err := LoadPrestoPublicKey(spkiPEM(t, &key.PublicKey))
	if err != nil {
		t.Fatalf("LoadPrestoPublicKey (SPKI): %v", err)
	}
	if pub.N.Cmp(key.PublicKey.N) != 0 {
		t.Fatal("loaded public key does not match original")
	}
}

func TestLoadPrestoPublicKey_RejectsTooSmall(t *testing.T) {
	key := genRSAKey(t, 1024)
	_, err := LoadPrestoPublicKey(spkiPEM(t, &key.PublicKey))
	if !errors.Is(err, ErrKeyTooSmall) {
		t.Fatalf("LoadPrestoPublicKey() error = %v, want ErrKeyTooSmall", err)
	}
}

func TestLoadPrestoPublicKey_GarbageInput(t *testing.T) {
	if _, err := LoadPrestoPublicKey([]byte("not a key")); err == nil {
		t.Fatal("expected an error for garbage input")
	}
}
