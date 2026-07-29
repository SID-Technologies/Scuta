package sigverify

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"testing"
)

func pemEncodeKeypair(t *testing.T, pub any, priv any) (pubPEM, privPEM []byte) {
	t.Helper()

	pubDER, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		t.Fatalf("marshaling public key: %v", err)
	}
	privDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatalf("marshaling private key: %v", err)
	}

	pubPEM = pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})
	privPEM = pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privDER})
	return pubPEM, privPEM
}

func TestSignVerify_Ed25519Roundtrip(t *testing.T) {
	pubPEM, privPEM, err := GenerateEd25519Keys()
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}

	data := []byte("tools:\n  pilum:\n    repo: sid-technologies/Pilum\n")
	sig, err := Sign(data, privPEM)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if err := Verify(data, sig, pubPEM); err != nil {
		t.Errorf("verify: %v", err)
	}
}

func TestSignVerify_RSARoundtrip(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa keygen: %v", err)
	}
	pubPEM, privPEM := pemEncodeKeypair(t, &priv.PublicKey, priv)

	data := []byte("payload")
	sig, err := Sign(data, privPEM)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if err := Verify(data, sig, pubPEM); err != nil {
		t.Errorf("verify: %v", err)
	}
}

func TestSignVerify_ECDSARoundtrip(t *testing.T) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ecdsa keygen: %v", err)
	}
	pubPEM, privPEM := pemEncodeKeypair(t, &priv.PublicKey, priv)

	data := []byte("payload")
	sig, err := Sign(data, privPEM)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if err := Verify(data, sig, pubPEM); err != nil {
		t.Errorf("verify: %v", err)
	}
}

func TestVerify_TamperedData(t *testing.T) {
	pubPEM, privPEM, err := GenerateEd25519Keys()
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}

	sig, err := Sign([]byte("original"), privPEM)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if err := Verify([]byte("tampered"), sig, pubPEM); err == nil {
		t.Error("expected verification failure for tampered data")
	}
}

func TestVerify_WrongKey(t *testing.T) {
	_, privPEM, err := GenerateEd25519Keys()
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	otherPub, _, err := GenerateEd25519Keys()
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}

	data := []byte("payload")
	sig, err := Sign(data, privPEM)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if err := Verify(data, sig, otherPub); err == nil {
		t.Error("expected verification failure with wrong key")
	}
}

func TestVerify_BadPEM(t *testing.T) {
	if err := Verify([]byte("data"), []byte("sig"), []byte("not a pem")); err == nil {
		t.Error("expected error for invalid PEM public key")
	}
}

func TestSign_BadPEM(t *testing.T) {
	if _, err := Sign([]byte("data"), []byte("not a pem")); err == nil {
		t.Error("expected error for invalid PEM private key")
	}
}
