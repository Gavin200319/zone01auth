package auth

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"testing"
)

func TestVerifySignature(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	pub := base64.StdEncoding.EncodeToString(der)

	nonce := NewNonce()
	sum := sha256.Sum256([]byte(nonce))
	sig, err := ecdsa.SignASN1(rand.Reader, key, sum[:])
	if err != nil {
		t.Fatal(err)
	}
	sigB64 := base64.StdEncoding.EncodeToString(sig)

	if err := VerifySignature(pub, nonce, sigB64); err != nil {
		t.Fatalf("valid signature rejected: %v", err)
	}
	if err := VerifySignature(pub, nonce+"x", sigB64); err == nil {
		t.Fatal("tampered payload accepted")
	}
	if err := VerifySignature(pub, nonce, "not-base64!!"); err == nil {
		t.Fatal("garbage signature accepted")
	}
}

func TestParsePublicKeyRejectsOtherCurves(t *testing.T) {
	key, _ := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	der, _ := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if _, err := ParsePublicKey(base64.StdEncoding.EncodeToString(der)); err == nil {
		t.Fatal("P-384 key accepted")
	}
}
