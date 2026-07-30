package installer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/sid-technologies/scuta/lib/sigverify"
)

func TestBundleManifestRoundTrip(t *testing.T) {
	manifest := &BundleManifest{
		Version: 1,
		Tools: map[string]BundleToolInfo{
			"pilum": {
				Version:  "1.0.0",
				Asset:    "pilum_1.0.0_linux_amd64.tar.gz",
				Checksum: "abc123",
			},
			"api-gen": {
				Version: "2.0.0",
				Asset:   "api-gen_2.0.0_linux_amd64.tar.gz",
			},
		},
		OS:   "linux",
		Arch: "amd64",
	}

	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var parsed BundleManifest
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if parsed.Version != 1 {
		t.Errorf("expected version 1, got %d", parsed.Version)
	}
	if len(parsed.Tools) != 2 {
		t.Errorf("expected 2 tools, got %d", len(parsed.Tools))
	}
	if parsed.Tools["pilum"].Version != "1.0.0" {
		t.Errorf("expected pilum 1.0.0, got %q", parsed.Tools["pilum"].Version)
	}
	if parsed.Tools["pilum"].Checksum != "abc123" {
		t.Errorf("expected checksum abc123, got %q", parsed.Tools["pilum"].Checksum)
	}
}

func TestBundleManifestV2RoundTrip(t *testing.T) {
	manifest := &BundleManifest{
		Version:   2,
		Platforms: []string{"darwin_arm64", "linux_amd64"},
		Tools: map[string]BundleToolInfo{
			"pilum": {
				Version: "1.0.0",
				Repo:    "sid-technologies/pilum",
				Assets: map[string]BundleAsset{
					"darwin_arm64": {Asset: "pilum_1.0.0_darwin_arm64.tar.gz", Checksum: "aaa"},
					"linux_amd64":  {Asset: "pilum_1.0.0_linux_amd64.tar.gz", Checksum: "bbb"},
				},
			},
		},
	}

	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var parsed BundleManifest
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if parsed.Version != 2 {
		t.Errorf("expected version 2, got %d", parsed.Version)
	}
	if len(parsed.Platforms) != 2 {
		t.Errorf("expected 2 platforms, got %v", parsed.Platforms)
	}
	got := parsed.Tools["pilum"].Assets["linux_amd64"]
	if got.Asset != "pilum_1.0.0_linux_amd64.tar.gz" || got.Checksum != "bbb" {
		t.Errorf("unexpected linux_amd64 asset: %+v", got)
	}
	if parsed.Tools["pilum"].Repo != "sid-technologies/pilum" {
		t.Errorf("expected repo to round-trip, got %q", parsed.Tools["pilum"].Repo)
	}
	// v1 fields must be omitted for v2 manifests.
	if strings.Contains(string(data), `"os"`) || strings.Contains(string(data), `"checksum": ""`) {
		t.Errorf("v2 manifest should omit empty v1 fields:\n%s", data)
	}
}

func TestCreateAndExtractBundle(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a fake source directory with a manifest and a fake asset
	srcDir := filepath.Join(tmpDir, "src")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create fake asset file
	assetContent := []byte("fake archive content")
	assetName := "mytool_1.0.0_linux_amd64.tar.gz"
	if err := os.WriteFile(filepath.Join(srcDir, assetName), assetContent, 0o644); err != nil {
		t.Fatal(err)
	}

	// Create manifest
	manifest := &BundleManifest{
		Version: 1,
		Tools: map[string]BundleToolInfo{
			"mytool": {
				Version: "1.0.0",
				Asset:   assetName,
			},
		},
		OS:   "linux",
		Arch: "amd64",
	}
	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, bundleManifestFile), manifestData, 0o644); err != nil {
		t.Fatal(err)
	}

	// Create bundle
	bundlePath := filepath.Join(tmpDir, "test-bundle.tar.gz")
	if err := createBundleTarGz(srcDir, bundlePath, manifest, false); err != nil {
		t.Fatalf("createBundleTarGz failed: %v", err)
	}

	// Verify bundle file exists
	info, err := os.Stat(bundlePath)
	if err != nil {
		t.Fatalf("bundle file not found: %v", err)
	}
	if info.Size() == 0 {
		t.Error("bundle file is empty")
	}

	// Extract bundle
	extractedManifest, extractDir, err := ExtractBundle(bundlePath)
	if err != nil {
		t.Fatalf("ExtractBundle failed: %v", err)
	}
	defer os.RemoveAll(extractDir)

	// Verify manifest
	if extractedManifest.Version != 1 {
		t.Errorf("expected version 1, got %d", extractedManifest.Version)
	}
	if len(extractedManifest.Tools) != 1 {
		t.Errorf("expected 1 tool, got %d", len(extractedManifest.Tools))
	}
	tool := extractedManifest.Tools["mytool"]
	if tool.Version != "1.0.0" {
		t.Errorf("expected version 1.0.0, got %q", tool.Version)
	}

	// Verify asset was extracted
	extractedAsset := filepath.Join(extractDir, assetName)
	data, err := os.ReadFile(extractedAsset)
	if err != nil {
		t.Fatalf("extracted asset not found: %v", err)
	}
	if string(data) != string(assetContent) {
		t.Errorf("asset content mismatch")
	}
}

// buildV2BundleDir lays out an extracted v2 bundle on disk: assets under
// their platform subdirectories, with real checksums in the manifest.
func buildV2BundleDir(t *testing.T, platforms map[string][]byte) (string, *BundleManifest) {
	t.Helper()
	dir := t.TempDir()

	keys := make([]string, 0, len(platforms))
	assets := make(map[string]BundleAsset, len(platforms))
	for key, content := range platforms {
		keys = append(keys, key)
		assetName := "mytool_1.0.0_" + key + ".tar.gz"
		assetDir := filepath.Join(dir, key)
		if err := os.MkdirAll(assetDir, 0o755); err != nil {
			t.Fatal(err)
		}
		assetPath := filepath.Join(assetDir, assetName)
		if err := os.WriteFile(assetPath, content, 0o644); err != nil {
			t.Fatal(err)
		}
		sum, err := fileSHA256(assetPath)
		if err != nil {
			t.Fatal(err)
		}
		assets[key] = BundleAsset{Asset: assetName, Checksum: sum}
	}

	manifest := &BundleManifest{
		Version:   2,
		Platforms: keys,
		Tools: map[string]BundleToolInfo{
			"mytool": {Version: "1.0.0", Repo: "example/mytool", Assets: assets},
		},
	}
	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, bundleManifestFile), manifestData, 0o644); err != nil {
		t.Fatal(err)
	}
	return dir, manifest
}

func TestCreateAndExtractBundleV2(t *testing.T) {
	hostKey := HostPlatform().Key()
	srcDir, manifest := buildV2BundleDir(t, map[string][]byte{
		hostKey:      []byte("host archive"),
		"plan9_mips": []byte("other archive"),
	})

	bundlePath := filepath.Join(t.TempDir(), "bundle-v2.tar.gz")
	if err := createBundleTarGz(srcDir, bundlePath, manifest, false); err != nil {
		t.Fatalf("createBundleTarGz failed: %v", err)
	}

	extracted, extractDir, err := ExtractBundle(bundlePath)
	if err != nil {
		t.Fatalf("ExtractBundle failed: %v", err)
	}
	defer os.RemoveAll(extractDir)

	if extracted.Version != 2 {
		t.Errorf("expected version 2, got %d", extracted.Version)
	}

	// Every platform asset must survive the round trip in its subdirectory.
	for key, asset := range extracted.Tools["mytool"].Assets {
		path := filepath.Join(extractDir, key, asset.Asset)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("asset %s missing after extract: %v", path, err)
		}
	}

	// And all checksums must verify.
	for _, res := range VerifyBundleChecksums(extractDir, extracted) {
		if res.Err != nil {
			t.Errorf("checksum verify failed for %s/%s: %v", res.Tool, res.Platform, res.Err)
		}
	}
}

func TestBundleAssetForHostV1(t *testing.T) {
	manifest := &BundleManifest{
		Version: 1,
		OS:      runtime.GOOS,
		Arch:    runtime.GOARCH,
		Tools: map[string]BundleToolInfo{
			"mytool": {Version: "1.0.0", Asset: "mytool.tar.gz", Checksum: "abc"},
		},
	}

	relPath, checksum, err := BundleAssetForHost(manifest, "mytool")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if relPath != "mytool.tar.gz" || checksum != "abc" {
		t.Errorf("got (%q, %q)", relPath, checksum)
	}

	if _, _, err := BundleAssetForHost(manifest, "missing"); err == nil {
		t.Error("expected error for unknown tool")
	}

	manifest.OS = "plan9"
	manifest.Arch = "mips"
	if _, _, err := BundleAssetForHost(manifest, "mytool"); err == nil {
		t.Error("expected error for platform mismatch")
	}
}

func TestBundleAssetForHostV2(t *testing.T) {
	hostKey := HostPlatform().Key()
	manifest := &BundleManifest{
		Version:   2,
		Platforms: []string{hostKey},
		Tools: map[string]BundleToolInfo{
			"mytool": {
				Version: "1.0.0",
				Assets: map[string]BundleAsset{
					hostKey: {Asset: "mytool_host.tar.gz", Checksum: "abc"},
				},
			},
			"otheros": {
				Version: "1.0.0",
				Assets: map[string]BundleAsset{
					"plan9_mips": {Asset: "otheros.tar.gz", Checksum: "def"},
				},
			},
		},
	}

	relPath, checksum, err := BundleAssetForHost(manifest, "mytool")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if relPath != filepath.Join(hostKey, "mytool_host.tar.gz") || checksum != "abc" {
		t.Errorf("got (%q, %q)", relPath, checksum)
	}

	if _, _, err := BundleAssetForHost(manifest, "otheros"); err == nil {
		t.Error("expected error when host platform is missing from tool assets")
	}
}

func TestVerifyBundleChecksums(t *testing.T) {
	dir, manifest := buildV2BundleDir(t, map[string][]byte{
		"linux_amd64":  []byte("linux content"),
		"darwin_arm64": []byte("darwin content"),
	})

	// Clean bundle: everything passes.
	for _, res := range VerifyBundleChecksums(dir, manifest) {
		if res.Err != nil {
			t.Errorf("expected pass for %s/%s, got %v", res.Tool, res.Platform, res.Err)
		}
	}

	// Tamper with one asset: exactly that platform fails.
	tampered := filepath.Join(dir, "linux_amd64", manifest.Tools["mytool"].Assets["linux_amd64"].Asset)
	if err := os.WriteFile(tampered, []byte("evil content"), 0o644); err != nil {
		t.Fatal(err)
	}
	failures := 0
	for _, res := range VerifyBundleChecksums(dir, manifest) {
		if res.Err != nil {
			failures++
			if res.Platform != "linux_amd64" {
				t.Errorf("unexpected failure on %s: %v", res.Platform, res.Err)
			}
		}
	}
	if failures != 1 {
		t.Errorf("expected exactly 1 failure, got %d", failures)
	}

	// Missing asset file.
	if err := os.Remove(tampered); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, res := range VerifyBundleChecksums(dir, manifest) {
		if res.Platform == "linux_amd64" {
			found = true
			if res.Err == nil || !strings.Contains(res.Err.Error(), "missing") {
				t.Errorf("expected missing-asset error, got %v", res.Err)
			}
		}
	}
	if !found {
		t.Error("missing asset was not reported at all")
	}
}

func TestVerifyBundleChecksumsV1(t *testing.T) {
	dir := t.TempDir()
	assetName := "mytool_1.0.0_linux_amd64.tar.gz"
	assetPath := filepath.Join(dir, assetName)
	if err := os.WriteFile(assetPath, []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}
	sum, err := fileSHA256(assetPath)
	if err != nil {
		t.Fatal(err)
	}

	manifest := &BundleManifest{
		Version: 1,
		OS:      "linux",
		Arch:    "amd64",
		Tools: map[string]BundleToolInfo{
			"mytool":     {Version: "1.0.0", Asset: assetName, Checksum: sum},
			"nochecksum": {Version: "1.0.0", Asset: assetName},
		},
	}

	for _, res := range VerifyBundleChecksums(dir, manifest) {
		switch res.Tool {
		case "mytool":
			if res.Err != nil {
				t.Errorf("expected pass, got %v", res.Err)
			}
		case "nochecksum":
			if res.Err == nil {
				t.Error("expected error for asset without a recorded checksum")
			}
		}
	}
}

func TestVerifyBundleSignature(t *testing.T) {
	pubPEM, privPEM, err := sigverify.GenerateEd25519Keys()
	if err != nil {
		t.Fatalf("keygen failed: %v", err)
	}

	dir := t.TempDir()
	manifestData := []byte(`{"version": 2, "tools": {}}`)
	if err := os.WriteFile(filepath.Join(dir, bundleManifestFile), manifestData, 0o644); err != nil {
		t.Fatal(err)
	}

	// Unsigned bundle, signature not required: ok, signed=false.
	signed, err := VerifyBundleSignature(dir, pubPEM, false)
	if err != nil || signed {
		t.Errorf("unsigned+optional: expected (false, nil), got (%v, %v)", signed, err)
	}

	// Unsigned bundle, signature required: error.
	if _, err := VerifyBundleSignature(dir, pubPEM, true); err == nil {
		t.Error("unsigned+required: expected error")
	}

	// Sign the manifest and verify.
	sig, err := sigverify.Sign(manifestData, privPEM)
	if err != nil {
		t.Fatalf("sign failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, bundleSignatureFile), sig, 0o644); err != nil {
		t.Fatal(err)
	}
	signed, err = VerifyBundleSignature(dir, pubPEM, true)
	if err != nil {
		t.Errorf("valid signature rejected: %v", err)
	}
	if !signed {
		t.Error("expected signed=true")
	}

	// Signed bundle but no public key configured: error even when optional.
	if _, err := VerifyBundleSignature(dir, nil, false); err == nil {
		t.Error("signed+no-key: expected error")
	}

	// Wrong key: error.
	otherPub, _, err := sigverify.GenerateEd25519Keys()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyBundleSignature(dir, otherPub, false); err == nil {
		t.Error("wrong key: expected error")
	}

	// Tampered manifest: error even with the right key.
	if err := os.WriteFile(filepath.Join(dir, bundleManifestFile), []byte(`{"version": 2, "tools": {"evil": {}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyBundleSignature(dir, pubPEM, false); err == nil {
		t.Error("tampered manifest: expected error")
	}
}

func TestCreateBundleTarGzIncludesSignature(t *testing.T) {
	srcDir, manifest := buildV2BundleDir(t, map[string][]byte{
		"linux_amd64": []byte("content"),
	})
	if err := os.WriteFile(filepath.Join(srcDir, bundleSignatureFile), []byte("fake signature"), 0o644); err != nil {
		t.Fatal(err)
	}

	bundlePath := filepath.Join(t.TempDir(), "signed.tar.gz")
	if err := createBundleTarGz(srcDir, bundlePath, manifest, true); err != nil {
		t.Fatalf("createBundleTarGz failed: %v", err)
	}

	_, extractDir, err := ExtractBundle(bundlePath)
	if err != nil {
		t.Fatalf("ExtractBundle failed: %v", err)
	}
	defer os.RemoveAll(extractDir)

	data, err := os.ReadFile(filepath.Join(extractDir, bundleSignatureFile))
	if err != nil {
		t.Fatalf("signature missing from bundle: %v", err)
	}
	if string(data) != "fake signature" {
		t.Error("signature content mismatch")
	}
}

func TestExtractBundleInvalidArchive(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a file that is not a valid tar.gz
	bundlePath := filepath.Join(tmpDir, "bad-bundle.tar.gz")
	if err := os.WriteFile(bundlePath, []byte("not a tar.gz"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, _, err := ExtractBundle(bundlePath)
	if err == nil {
		t.Error("expected error for invalid archive, got nil")
	}
}
