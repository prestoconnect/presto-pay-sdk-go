package crypto

import (
	"crypto/rand"
	"crypto/rsa"
	"testing"
)

func testKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	return key
}

func TestSignVerify_RoundTrip(t *testing.T) {
	key := testKey(t)
	const canonical = "1200:MYR:Order #12345:PW2401XH9KCX"

	sig, err := Sign(key, canonical)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if err := Verify(&key.PublicKey, canonical, sig); err != nil {
		t.Fatalf("Verify: %v", err)
	}
}

func TestVerify_WrongKeyFails(t *testing.T) {
	key := testKey(t)
	other := testKey(t)
	const canonical = "a:b:c"

	sig, err := Sign(key, canonical)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if err := Verify(&other.PublicKey, canonical, sig); err == nil {
		t.Fatal("expected verification to fail under the wrong key")
	}
}

func TestVerify_TamperedCanonicalFails(t *testing.T) {
	key := testKey(t)
	sig, err := Sign(key, "a:b:c")
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if err := Verify(&key.PublicKey, "a:b:d", sig); err == nil {
		t.Fatal("expected verification to fail for a tampered canonical string")
	}
}

func TestVerify_InvalidBase64IsVerificationFailure(t *testing.T) {
	key := testKey(t)
	err := Verify(&key.PublicKey, "a:b:c", "not-valid-base64!!!")
	if err != ErrInvalidSignature {
		t.Fatalf("Verify() error = %v, want ErrInvalidSignature", err)
	}
}

func TestVerify_NonCanonicalPaddingFails(t *testing.T) {
	key := testKey(t)
	// "QQ=" is one padding character short of "QQ==" (the correct encoding
	// of a single byte): a lenient decoder might still accept it, a strict
	// one must not.
	if err := Verify(&key.PublicKey, "a:b:c", "QQ="); err != ErrInvalidSignature {
		t.Fatalf("Verify() error = %v, want ErrInvalidSignature for non-canonical padding", err)
	}
}
