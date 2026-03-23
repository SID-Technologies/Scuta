package cve

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestConvertVulns(t *testing.T) {
	input := []osvVuln{
		{
			ID:      "GHSA-abc123",
			Summary: "A critical vulnerability",
			Aliases: []string{"CVE-2026-1234"},
			Severity: []osvSeverity{
				{Type: "CVSS_V3", Score: "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H"},
			},
		},
		{
			ID:      "GO-2026-5678",
			Summary: "A moderate vulnerability",
		},
	}

	result := convertVulns(input)

	if len(result) != 2 {
		t.Fatalf("expected 2 vulns, got %d", len(result))
	}

	if result[0].ID != "GHSA-abc123" {
		t.Errorf("expected GHSA-abc123, got %q", result[0].ID)
	}
	if result[0].Severity == "" {
		t.Error("expected severity to be set")
	}
	if len(result[0].Aliases) != 1 || result[0].Aliases[0] != "CVE-2026-1234" {
		t.Errorf("unexpected aliases: %v", result[0].Aliases)
	}

	if result[1].Severity != "" {
		t.Errorf("expected empty severity for second vuln, got %q", result[1].Severity)
	}
}

func TestCacheRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()

	c := cache{
		"pilum@1.0.0": cacheEntry{
			Vulns:     []Vuln{{ID: "CVE-2026-1", Summary: "test vuln"}},
			CheckedAt: time.Now(),
		},
	}

	if err := saveCache(tmpDir, c); err != nil {
		t.Fatalf("saveCache failed: %v", err)
	}

	loaded, err := loadCache(tmpDir)
	if err != nil {
		t.Fatalf("loadCache failed: %v", err)
	}

	entry, ok := loaded["pilum@1.0.0"]
	if !ok {
		t.Fatal("expected cache entry for pilum@1.0.0")
	}
	if len(entry.Vulns) != 1 || entry.Vulns[0].ID != "CVE-2026-1" {
		t.Errorf("unexpected cached vulns: %v", entry.Vulns)
	}
}

func TestCacheHit(t *testing.T) {
	tmpDir := t.TempDir()

	// Pre-populate cache with recent entry
	c := cache{
		"mytool@1.0.0": cacheEntry{
			Vulns:     nil, // no vulns
			CheckedAt: time.Now(),
		},
	}
	if err := saveCache(tmpDir, c); err != nil {
		t.Fatal(err)
	}

	// CheckWithCache should return cached result without hitting the API
	vulns, err := CheckWithCache(tmpDir, "mytool", "1.0.0", "Go")
	if err != nil {
		t.Fatalf("CheckWithCache failed: %v", err)
	}

	if vulns != nil {
		t.Errorf("expected nil vulns from cache, got %d", len(vulns))
	}
}

func TestLoadCacheEmpty(t *testing.T) {
	tmpDir := t.TempDir()

	c, err := loadCache(tmpDir)
	if err != nil {
		t.Fatalf("loadCache failed: %v", err)
	}

	if c != nil {
		t.Errorf("expected nil cache, got %v", c)
	}
}

func TestLoadCacheMalformed(t *testing.T) {
	tmpDir := t.TempDir()

	fp := filepath.Join(tmpDir, cacheFile)
	if err := os.WriteFile(fp, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := loadCache(tmpDir)
	if err == nil {
		t.Error("expected error for malformed cache, got nil")
	}
}

func TestConvertVulnsEmpty(t *testing.T) {
	result := convertVulns(nil)
	if result != nil {
		t.Errorf("expected nil for empty input, got %v", result)
	}
}

func TestOsvQueryRequestFormat(t *testing.T) {
	req := osvQueryRequest{
		Package: osvPackage{
			Name:      "github.com/example/tool",
			Ecosystem: "Go",
		},
		Version: "1.2.3",
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatal(err)
	}

	pkg := parsed["package"].(map[string]interface{})
	if pkg["name"] != "github.com/example/tool" {
		t.Errorf("unexpected name: %v", pkg["name"])
	}
	if pkg["ecosystem"] != "Go" {
		t.Errorf("unexpected ecosystem: %v", pkg["ecosystem"])
	}
}
