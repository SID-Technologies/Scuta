package installer

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"

	"github.com/sid-technologies/scuta/lib/github"
)

func TestVerifySignatureRSA(t *testing.T) {
	// Generate RSA key pair
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	pubKeyBytes, err := x509.MarshalPKIXPublicKey(&privKey.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	pubKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubKeyBytes})

	tmpDir := t.TempDir()

	// Write a file to sign
	filePath := filepath.Join(tmpDir, "binary")
	fileContent := []byte("this is the binary content")
	if err := os.WriteFile(filePath, fileContent, 0o644); err != nil {
		t.Fatal(err)
	}

	// Sign it
	hash := sha256.Sum256(fileContent)
	sig, err := rsa.SignPKCS1v15(rand.Reader, privKey, crypto.SHA256, hash[:])
	if err != nil {
		t.Fatal(err)
	}

	sigPath := filepath.Join(tmpDir, "binary.sig")
	if err := os.WriteFile(sigPath, sig, 0o644); err != nil {
		t.Fatal(err)
	}

	// Verify
	if err := VerifySignature(filePath, sigPath, pubKeyPEM); err != nil {
		t.Fatalf("valid RSA signature should pass: %v", err)
	}
}

func TestVerifySignatureECDSA(t *testing.T) {
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	pubKeyBytes, err := x509.MarshalPKIXPublicKey(&privKey.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	pubKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubKeyBytes})

	tmpDir := t.TempDir()

	filePath := filepath.Join(tmpDir, "binary")
	fileContent := []byte("ecdsa test content")
	if err := os.WriteFile(filePath, fileContent, 0o644); err != nil {
		t.Fatal(err)
	}

	hash := sha256.Sum256(fileContent)
	sig, err := ecdsa.SignASN1(rand.Reader, privKey, hash[:])
	if err != nil {
		t.Fatal(err)
	}

	sigPath := filepath.Join(tmpDir, "binary.sig")
	if err := os.WriteFile(sigPath, sig, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := VerifySignature(filePath, sigPath, pubKeyPEM); err != nil {
		t.Fatalf("valid ECDSA signature should pass: %v", err)
	}
}

func TestVerifySignatureInvalid(t *testing.T) {
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	pubKeyBytes, err := x509.MarshalPKIXPublicKey(&privKey.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	pubKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubKeyBytes})

	tmpDir := t.TempDir()

	filePath := filepath.Join(tmpDir, "binary")
	if err := os.WriteFile(filePath, []byte("original content"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Write a bogus signature
	sigPath := filepath.Join(tmpDir, "binary.sig")
	if err := os.WriteFile(sigPath, []byte("invalid signature data"), 0o644); err != nil {
		t.Fatal(err)
	}

	err = VerifySignature(filePath, sigPath, pubKeyPEM)
	if err == nil {
		t.Error("expected error for invalid signature, got nil")
	}
}

func TestVerifySignatureBadPEM(t *testing.T) {
	tmpDir := t.TempDir()

	filePath := filepath.Join(tmpDir, "binary")
	if err := os.WriteFile(filePath, []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}

	sigPath := filepath.Join(tmpDir, "binary.sig")
	if err := os.WriteFile(sigPath, []byte("sig"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := VerifySignature(filePath, sigPath, []byte("not a valid PEM"))
	if err == nil {
		t.Error("expected error for bad PEM, got nil")
	}
}

func TestFindSignatureAsset(t *testing.T) {
	assets := []github.Asset{
		{Name: "tool_linux_amd64.tar.gz"},
		{Name: "tool_linux_amd64.tar.gz.sig"},
		{Name: "checksums.txt"},
	}

	sig := FindSignatureAsset(assets, "tool_linux_amd64.tar.gz")
	if sig == nil {
		t.Fatal("expected to find .sig asset")
	}
	if sig.Name != "tool_linux_amd64.tar.gz.sig" {
		t.Errorf("expected tool_linux_amd64.tar.gz.sig, got %q", sig.Name)
	}
}

func TestFindSignatureAssetMissing(t *testing.T) {
	assets := []github.Asset{
		{Name: "tool_linux_amd64.tar.gz"},
		{Name: "checksums.txt"},
	}

	sig := FindSignatureAsset(assets, "tool_linux_amd64.tar.gz")
	if sig != nil {
		t.Error("expected nil for missing .sig asset")
	}
}
