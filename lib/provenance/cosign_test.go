package provenance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// found / notFound stub exec.LookPath.
func found(string) (string, error)    { return "/usr/bin/stub", nil }
func notFound(string) (string, error) { return "", errors.New("not found") }

// recordingRunner captures the CLI invocation and returns canned output.
type recordingRunner struct {
	name string
	args []string
	out  []byte
	err  error
}

func (r *recordingRunner) run(_ context.Context, name string, args ...string) ([]byte, error) {
	r.name = name
	r.args = args
	return r.out, r.err
}

// stubDownload writes canned bytes per URL.
func stubDownload(t *testing.T, contents map[string][]byte) func(context.Context, string, string) error {
	t.Helper()
	return func(_ context.Context, url, dest string) error {
		data, ok := contents[url]
		if !ok {
			return errors.New("unexpected download: " + url)
		}
		return os.WriteFile(dest, data, 0o600)
	}
}

// writeAsset creates the downloaded asset on disk and returns its request skeleton.
func writeAsset(t *testing.T, content []byte) Request {
	t.Helper()
	dir := t.TempDir()
	assetPath := filepath.Join(dir, "tool_1.0.0_linux_amd64.tar.gz")
	if err := os.WriteFile(assetPath, content, 0o600); err != nil {
		t.Fatal(err)
	}
	return Request{
		Repo:      "owner/tool",
		Tag:       "v1.0.0",
		AssetName: "tool_1.0.0_linux_amd64.tar.gz",
		AssetPath: assetPath,
		WorkDir:   dir,
	}
}

func hasArgPair(args []string, flag, value string) bool {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == flag && args[i+1] == value {
			return true
		}
	}
	return false
}

func hasFlag(args []string, flag string) bool {
	for _, a := range args {
		if a == flag {
			return true
		}
	}
	return false
}

func TestCosign_SkipsWhenCLIMissing(t *testing.T) {
	c := &Cosign{lookPath: notFound}

	res, err := c.Verify(context.Background(), writeAsset(t, []byte("bytes")))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !res.Skipped || !strings.Contains(res.Reason, "not found") {
		t.Fatalf("expected PATH skip, got %+v", res)
	}
}

func TestCosign_SkipsWhenNoMaterial(t *testing.T) {
	c := &Cosign{lookPath: found}
	req := writeAsset(t, []byte("bytes"))
	req.Assets = []Asset{{Name: req.AssetName}, {Name: "checksums.txt"}}

	res, err := c.Verify(context.Background(), req)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !res.Skipped || !strings.Contains(res.Reason, "no cosign signature material") {
		t.Fatalf("expected material skip, got %+v", res)
	}
}

func TestCosign_BundleHappyPath(t *testing.T) {
	runner := &recordingRunner{out: []byte("Verified OK")}
	c := &Cosign{lookPath: found, run: runner.run}

	req := writeAsset(t, []byte("bytes"))
	bundleName := req.AssetName + ".sigstore.json"
	req.Assets = []Asset{
		{Name: req.AssetName},
		{Name: bundleName, URL: "https://example.com/bundle"},
	}
	req.Download = stubDownload(t, map[string][]byte{
		"https://example.com/bundle": []byte("{}"),
	})

	res, err := c.Verify(context.Background(), req)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !res.Verified {
		t.Fatalf("expected verified, got %+v", res)
	}

	if runner.name != "cosign" || runner.args[0] != "verify-blob" {
		t.Fatalf("unexpected invocation: %s %v", runner.name, runner.args)
	}
	if !hasArgPair(runner.args, "--bundle", filepath.Join(req.WorkDir, bundleName)) {
		t.Fatalf("missing --bundle arg: %v", runner.args)
	}
	if !hasFlag(runner.args, "--new-bundle-format") {
		t.Fatalf("expected --new-bundle-format for .sigstore.json: %v", runner.args)
	}
	if !hasArgPair(runner.args, "--certificate-oidc-issuer", defaultOIDCIssuer) {
		t.Fatalf("missing default issuer: %v", runner.args)
	}
	if !hasArgPair(runner.args, "--certificate-identity-regexp", `^https://github\.com/owner/tool/`) {
		t.Fatalf("identity regexp not derived from repo: %v", runner.args)
	}
	if runner.args[len(runner.args)-1] != req.AssetPath {
		t.Fatalf("blob path should be the asset itself: %v", runner.args)
	}
}

func TestCosign_SigCertHappyPath(t *testing.T) {
	runner := &recordingRunner{}
	c := &Cosign{
		identity: CosignIdentity{IdentityRegexp: "^https://example.com/ci$", OIDCIssuer: "https://issuer.example"},
		lookPath: found,
		run:      runner.run,
	}

	req := writeAsset(t, []byte("bytes"))
	req.Assets = []Asset{
		{Name: req.AssetName},
		{Name: req.AssetName + ".sig", URL: "https://example.com/sig"},
		{Name: req.AssetName + ".pem", URL: "https://example.com/pem"},
	}
	req.Download = stubDownload(t, map[string][]byte{
		"https://example.com/sig": []byte("sig"),
		"https://example.com/pem": []byte("pem"),
	})

	res, err := c.Verify(context.Background(), req)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !res.Verified {
		t.Fatalf("expected verified, got %+v", res)
	}
	if !hasArgPair(runner.args, "--certificate-identity-regexp", "^https://example.com/ci$") {
		t.Fatalf("identity override not honored: %v", runner.args)
	}
	if !hasArgPair(runner.args, "--certificate-oidc-issuer", "https://issuer.example") {
		t.Fatalf("issuer override not honored: %v", runner.args)
	}
	if !hasArgPair(runner.args, "--signature", filepath.Join(req.WorkDir, req.AssetName+".sig")) {
		t.Fatalf("missing --signature: %v", runner.args)
	}
}

func TestCosign_BareSigWithoutCertIsNotCosignMaterial(t *testing.T) {
	c := &Cosign{lookPath: found}
	req := writeAsset(t, []byte("bytes"))
	req.Assets = []Asset{
		{Name: req.AssetName},
		{Name: req.AssetName + ".sig"},
	}

	res, err := c.Verify(context.Background(), req)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !res.Skipped {
		t.Fatalf("bare .sig is the key-based path, not cosign keyless: %+v", res)
	}
}

func TestCosign_SignedChecksumsVouchesForAsset(t *testing.T) {
	content := []byte("asset bytes")
	sum := sha256.Sum256(content)
	checksums := hex.EncodeToString(sum[:]) + "  tool_1.0.0_linux_amd64.tar.gz\n"

	runner := &recordingRunner{}
	c := &Cosign{lookPath: found, run: runner.run}

	req := writeAsset(t, content)
	req.Assets = []Asset{
		{Name: req.AssetName},
		{Name: "checksums.txt", URL: "https://example.com/sums"},
		{Name: "checksums.txt.sig", URL: "https://example.com/sums.sig"},
		{Name: "checksums.txt.pem", URL: "https://example.com/sums.pem"},
	}
	req.Download = stubDownload(t, map[string][]byte{
		"https://example.com/sums":     []byte(checksums),
		"https://example.com/sums.sig": []byte("sig"),
		"https://example.com/sums.pem": []byte("pem"),
	})

	res, err := c.Verify(context.Background(), req)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !res.Verified || !strings.Contains(res.Detail, "checksums-signature") {
		t.Fatalf("expected checksums-signature verification, got %+v", res)
	}
}

func TestCosign_SignedChecksumsMismatchFails(t *testing.T) {
	checksums := strings.Repeat("0", 64) + "  tool_1.0.0_linux_amd64.tar.gz\n"

	runner := &recordingRunner{}
	c := &Cosign{lookPath: found, run: runner.run}

	req := writeAsset(t, []byte("asset bytes"))
	req.Assets = []Asset{
		{Name: req.AssetName},
		{Name: "checksums.txt", URL: "https://example.com/sums"},
		{Name: "checksums.txt.sig", URL: "https://example.com/sums.sig"},
		{Name: "checksums.txt.pem", URL: "https://example.com/sums.pem"},
	}
	req.Download = stubDownload(t, map[string][]byte{
		"https://example.com/sums":     []byte(checksums),
		"https://example.com/sums.sig": []byte("sig"),
		"https://example.com/sums.pem": []byte("pem"),
	})

	_, err := c.Verify(context.Background(), req)
	if err == nil || !strings.Contains(err.Error(), "does not vouch") {
		t.Fatalf("expected vouch failure, got %v", err)
	}
}

func TestCosign_SignedChecksumsMissingEntryFails(t *testing.T) {
	checksums := strings.Repeat("0", 64) + "  other_asset.tar.gz\n"

	runner := &recordingRunner{}
	c := &Cosign{lookPath: found, run: runner.run}

	req := writeAsset(t, []byte("asset bytes"))
	req.Assets = []Asset{
		{Name: req.AssetName},
		{Name: "checksums.txt", URL: "https://example.com/sums"},
		{Name: "checksums.txt.sig", URL: "https://example.com/sums.sig"},
		{Name: "checksums.txt.pem", URL: "https://example.com/sums.pem"},
	}
	req.Download = stubDownload(t, map[string][]byte{
		"https://example.com/sums":     []byte(checksums),
		"https://example.com/sums.sig": []byte("sig"),
		"https://example.com/sums.pem": []byte("pem"),
	})

	_, err := c.Verify(context.Background(), req)
	if err == nil || !strings.Contains(err.Error(), "no entry") {
		t.Fatalf("expected missing-entry failure, got %v", err)
	}
}

func TestCosign_RejectionIsFatal(t *testing.T) {
	runner := &recordingRunner{out: []byte("Error: invalid signature"), err: errors.New("exit status 1")}
	c := &Cosign{lookPath: found, run: runner.run}

	req := writeAsset(t, []byte("bytes"))
	req.Assets = []Asset{
		{Name: req.AssetName},
		{Name: req.AssetName + ".bundle", URL: "https://example.com/bundle"},
	}
	req.Download = stubDownload(t, map[string][]byte{
		"https://example.com/bundle": []byte("{}"),
	})

	_, err := c.Verify(context.Background(), req)
	if err == nil || !strings.Contains(err.Error(), "cosign rejected") {
		t.Fatalf("expected rejection error, got %v", err)
	}
}

func TestCosign_NoRepoAndNoOverrideSkips(t *testing.T) {
	c := &Cosign{lookPath: found}
	req := writeAsset(t, []byte("bytes"))
	req.Repo = ""
	req.Assets = []Asset{
		{Name: req.AssetName},
		{Name: req.AssetName + ".bundle", URL: "https://example.com/bundle"},
	}

	res, err := c.Verify(context.Background(), req)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !res.Skipped || !strings.Contains(res.Reason, "signer identity") {
		t.Fatalf("expected identity skip, got %+v", res)
	}
}
