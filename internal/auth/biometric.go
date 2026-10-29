package auth

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"strings"
)

var ErrBadSignature = errors.New("biometric signature verification failed")

func decodeB64(s string) ([]byte, error) {
	s = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == ' ' || r == '\t' {
			return -1
		}
		return r
	}, s)
	for _, enc := range []*base64.Encoding{
		base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding,
	} {
		if b, err := enc.DecodeString(s); err == nil {
			return b, nil
		}
	}
	return nil, errors.New("invalid base64")
}

func ParsePublicKey(b64 string) (*ecdsa.PublicKey, error) {
	der, err := decodeB64(b64)
	if err != nil {
		return nil, err
	}
	k, err := x509.ParsePKIXPublicKey(der)
	if err != nil {
		return nil, errors.New("public key must be a base64 X.509 SubjectPublicKeyInfo")
	}
	pk, ok := k.(*ecdsa.PublicKey)
	if !ok || pk.Curve != elliptic.P256() {
		return nil, errors.New("public key must be an EC P-256 key")
	}
	return pk, nil
}

func VerifySignature(publicKeyB64, payload, signatureB64 string) error {
	pk, err := ParsePublicKey(publicKeyB64)
	if err != nil {
		return err
	}
	sig, err := decodeB64(signatureB64)
	if err != nil {
		return ErrBadSignature
	}
	sum := sha256.Sum256([]byte(payload))
	if !ecdsa.VerifyASN1(pk, sum[:], sig) {
		return ErrBadSignature
	}
	return nil
}
