package config

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMergeConfigPriority(t *testing.T) {
	dst := DefaultConfig()
	src := Config{
		UpdateInterval: "12h",
		RegistryURL:    "https://custom.example.com/registry",
	}

	mergeConfig(&dst, src)

	if dst.UpdateInterval != "12h" {
		t.Errorf("expected UpdateInterval '12h', got %q", dst.UpdateInterval)
	}
	if dst.RegistryURL != "https://custom.example.com/registry" {
		t.Errorf("expected custom registry URL, got %q", dst.RegistryURL)
	}
}

func TestMergeConfigDoesNotOverrideWithEmpty(t *testing.T) {
	dst := Config{
		UpdateInterval: "12h",
		GithubToken:    "my-token",
	}
	src := Config{
		UpdateInterval: "", // empty should NOT override
	}

	mergeConfig(&dst, src)

	if dst.UpdateInterval != "12h" {
		t.Errorf("empty src should not override dst: got %q", dst.UpdateInterval)
	}
	if dst.GithubToken != "my-token" {
		t.Errorf("unset src should not override dst: got %q", dst.GithubToken)
	}
}

func TestLoadWithMergeLocalOnly(t *testing.T) {
	tmpDir := t.TempDir()

	// Write local config
	localCfg := Config{
		Version:        1,
		UpdateInterval: "6h",
	}
	if err := Save(tmpDir, localCfg); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadWithMerge(tmpDir)
	if err != nil {
		t.Fatalf("LoadWithMerge failed: %v", err)
	}

	if cfg.UpdateInterval != "6h" {
		t.Errorf("expected 6h, got %q", cfg.UpdateInterval)
	}
}

func TestLoadWithMergeSystemConfig(t *testing.T) {
	// This test checks mergeConfig behavior, not actual /etc/scuta/ loading
	// since we can't write to /etc in tests.
	dst := DefaultConfig()

	systemCfg := Config{
		UpdateInterval: "48h",
		PolicyURL:      "https://corp.example.com/policy.yaml",
	}
	mergeConfig(&dst, systemCfg)

	localCfg := Config{
		UpdateInterval: "12h", // local overrides system
	}
	mergeConfig(&dst, localCfg)

	if dst.UpdateInterval != "12h" {
		t.Errorf("local should override system: got %q", dst.UpdateInterval)
	}
	if dst.PolicyURL != "https://corp.example.com/policy.yaml" {
		t.Errorf("system policy URL should persist: got %q", dst.PolicyURL)
	}
}

func TestFetchRemoteConfig(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/x-yaml")
		_, _ = w.Write([]byte("update_interval: 48h\nregistry_url: https://corp.example.com/registry\n"))
	}))
	defer server.Close()

	tmpDir := t.TempDir()

	cfg, err := fetchRemoteConfig(tmpDir, server.URL)
	if err != nil {
		t.Fatalf("fetchRemoteConfig failed: %v", err)
	}

	if cfg.UpdateInterval != "48h" {
		t.Errorf("expected 48h, got %q", cfg.UpdateInterval)
	}
	if cfg.RegistryURL != "https://corp.example.com/registry" {
		t.Errorf("expected corporate registry, got %q", cfg.RegistryURL)
	}

	// Verify cache file was created
	cachePath := filepath.Join(tmpDir, remoteConfigCache)
	if _, err := os.Stat(cachePath); os.IsNotExist(err) {
		t.Error("expected cache file to be created")
	}
}

func TestFetchRemoteConfigUsesCache(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		callCount++
		_, _ = w.Write([]byte("update_interval: 48h\n"))
	}))
	defer server.Close()

	tmpDir := t.TempDir()

	// First fetch
	_, err := fetchRemoteConfig(tmpDir, server.URL)
	if err != nil {
		t.Fatal(err)
	}

	// Second fetch should hit cache
	cfg, err := fetchRemoteConfig(tmpDir, server.URL)
	if err != nil {
		t.Fatal(err)
	}

	if callCount != 1 {
		t.Errorf("expected 1 HTTP call (cache hit), got %d", callCount)
	}

	if cfg.UpdateInterval != "48h" {
		t.Errorf("expected 48h from cache, got %q", cfg.UpdateInterval)
	}
}

func TestMergeConfigAuditLogDestination(t *testing.T) {
	dst := DefaultConfig()
	src := Config{
		AuditLogDestination: "syslog",
	}

	mergeConfig(&dst, src)

	if dst.AuditLogDestination != "syslog" {
		t.Errorf("expected 'syslog', got %q", dst.AuditLogDestination)
	}
}

func TestMergeConfigAuditLogDestinationNoOverrideWithEmpty(t *testing.T) {
	dst := Config{
		AuditLogDestination: "syslog",
	}
	src := Config{
		AuditLogDestination: "", // empty should NOT override
	}

	mergeConfig(&dst, src)

	if dst.AuditLogDestination != "syslog" {
		t.Errorf("empty src should not override: got %q", dst.AuditLogDestination)
	}
}

func TestMergeConfigBooleanFields(t *testing.T) {
	// Booleans: true in src should override false in dst
	dst := DefaultConfig()
	src := Config{
		Telemetry:        true,
		RequireSignature: true,
	}

	mergeConfig(&dst, src)

	if !dst.Telemetry {
		t.Error("expected Telemetry=true after merge")
	}
	if !dst.RequireSignature {
		t.Error("expected RequireSignature=true after merge")
	}
}

func TestLoadWithMergeAuditLogDestination(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := Config{
		Version:             1,
		AuditLogDestination: "stdout",
	}
	if err := Save(tmpDir, cfg); err != nil {
		t.Fatal(err)
	}

	merged, err := LoadWithMerge(tmpDir)
	if err != nil {
		t.Fatalf("LoadWithMerge failed: %v", err)
	}

	if merged.AuditLogDestination != "stdout" {
		t.Errorf("expected 'stdout', got %q", merged.AuditLogDestination)
	}
}

func TestFetchRemoteConfigFallbackToCache(t *testing.T) {
	tmpDir := t.TempDir()

	// Pre-populate cache with a past mod time (expired TTL)
	cachePath := filepath.Join(tmpDir, remoteConfigCache)
	if err := os.WriteFile(cachePath, []byte("update_interval: 72h\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Set mod time to the past so cache is expired
	past := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(cachePath, past, past); err != nil {
		t.Fatal(err)
	}

	// The fetch will fail because the URL is unreachable, but should fall back to cache
	cfg, err := fetchRemoteConfig(tmpDir, "https://unreachable.invalid/config.yaml")
	if err != nil {
		t.Fatal(err) // should succeed via cache fallback
	}

	if cfg.UpdateInterval != "72h" {
		t.Errorf("expected 72h from fallback cache, got %q", cfg.UpdateInterval)
	}
}
