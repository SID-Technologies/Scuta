// Package installer handles downloading, verifying, and installing tool binaries.
package installer

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"os"
	"strings"

	"github.com/sid-technologies/scuta/lib/errors"
	"github.com/sid-technologies/scuta/lib/github"
	"github.com/sid-technologies/scuta/lib/output"
)

// VerifySignature verifies a detached signature (.sig) against a file using a PEM-encoded public key.
// Supports RSA, ECDSA, and Ed25519 keys.
func VerifySignature(filePath string, signaturePath string, publicKeyPEM []byte) error {
	// Read the file to verify
	fileData, err := os.ReadFile(filePath)
	if err != nil {
		return errors.Wrap(err, "reading file for signature verification")
	}

	// Read the signature
	sigData, err := os.ReadFile(signaturePath)
	if err != nil {
		return errors.Wrap(err, "reading signature file")
	}

	// Parse the public key
	block, _ := pem.Decode(publicKeyPEM)
	if block == nil {
		return errors.New("failed to decode PEM public key")
	}

	pubKey, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return errors.Wrap(err, "parsing public key")
	}

	// Hash the file content
	hash := sha256.Sum256(fileData)

	// Verify based on key type
	switch key := pubKey.(type) {
	case *rsa.PublicKey:
		if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, hash[:], sigData); err != nil {
			return errors.New("RSA signature verification failed")
		}
	case *ecdsa.PublicKey:
		if !ecdsa.VerifyASN1(key, hash[:], sigData) {
			return errors.New("ECDSA signature verification failed")
		}
	case ed25519.PublicKey:
		if !ed25519.Verify(key, fileData, sigData) {
			return errors.New("Ed25519 signature verification failed")
		}
	default:
		return errors.New("unsupported public key type: %T", pubKey)
	}

	return nil
}

// FindSignatureAsset looks for a .sig file matching the asset name in the release.
func FindSignatureAsset(assets []github.Asset, assetName string) *github.Asset {
	sigName := assetName + ".sig"
	for i := range assets {
		if strings.EqualFold(assets[i].Name, sigName) {
			return &assets[i]
		}
	}
	return nil
}

// DownloadAndVerifySignature downloads the .sig file and verifies the asset signature.
// Returns nil if no .sig is found and requireSignature is false.
// Returns an error if no .sig is found and requireSignature is true.
func DownloadAndVerifySignature(
	ctx context.Context,
	ghClient *github.Client,
	release *github.Release,
	assetName string,
	assetPath string,
	publicKeyPEM []byte,
	requireSignature bool,
) error {
	sigAsset := FindSignatureAsset(release.Assets, assetName)
	if sigAsset == nil {
		if requireSignature {
			return errors.New("signature required but no .sig file found for %s", assetName)
		}
		output.Debugf("No .sig file found for %s, skipping signature verification", assetName)
		return nil
	}

	// Download the signature to a temp file
	sigPath := assetPath + ".sig"
	if err := ghClient.DownloadAsset(ctx, sigAsset.BrowserDownloadURL, sigPath); err != nil {
		return errors.Wrap(err, "downloading signature for %s", assetName)
	}
	defer os.Remove(sigPath)

	if err := VerifySignature(assetPath, sigPath, publicKeyPEM); err != nil {
		return errors.Wrap(err, "signature verification failed for %s", assetName)
	}

	output.Debugf("Signature verified for %s", assetName)
	return nil
}
