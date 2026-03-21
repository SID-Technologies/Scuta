package installer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
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
	if err := createBundleTarGz(srcDir, bundlePath, manifest); err != nil {
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
