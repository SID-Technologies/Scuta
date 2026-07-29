// Package installer handles downloading, verifying, and installing tool binaries.
package installer

import (
	"context"
	"os"
	"strings"

	"github.com/sid-technologies/scuta/lib/errors"
	"github.com/sid-technologies/scuta/lib/github"
	"github.com/sid-technologies/scuta/lib/output"
	"github.com/sid-technologies/scuta/lib/sigverify"
)

// VerifySignature verifies a detached signature (.sig) against a file using a PEM-encoded public key.
// Supports RSA, ECDSA, and Ed25519 keys (see lib/sigverify for the primitives).
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

	return sigverify.Verify(fileData, sigData, publicKeyPEM)
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
