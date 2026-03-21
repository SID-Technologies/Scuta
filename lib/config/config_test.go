package config

import (
	"testing"
)

func TestLoadDefault(t *testing.T) {
	tmpDir := t.TempDir()

	cfg, err := Load(tmpDir)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.Version != CurrentConfigVersion {
		t.Errorf("expected version %d, got %d", CurrentConfigVersion, cfg.Version)
	}
	if cfg.UpdateInterval != "24h" {
		t.Errorf("expected default update_interval '24h', got %q", cfg.UpdateInterval)
	}
}

func TestSaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := Config{
		Version:        1,
		UpdateInterval: "12h",
		GithubToken:    "ghp_test123",
		RegistryURL:    "https://example.com/registry.yaml",
	}

	if err := Save(tmpDir, cfg); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := Load(tmpDir)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if loaded.UpdateInterval != "12h" {
		t.Errorf("expected '12h', got %q", loaded.UpdateInterval)
	}
	if loaded.GithubToken != "ghp_test123" {
		t.Errorf("expected token to persist, got %q", loaded.GithubToken)
	}
	if loaded.RegistryURL != "https://example.com/registry.yaml" {
		t.Errorf("expected registry URL to persist, got %q", loaded.RegistryURL)
	}
}

func TestValidKeysContainsAll(t *testing.T) {
	keys := ValidKeys()
	expected := []string{
		"update_interval", "github_token", "registry_url",
		"github_base_url", "policy_url", "config_url",
		"telemetry", "require_signature", "signature_public_key",
		"audit_log_destination",
	}

	keySet := make(map[string]bool, len(keys))
	for _, k := range keys {
		keySet[k] = true
	}

	for _, exp := range expected {
		if !keySet[exp] {
			t.Errorf("ValidKeys() missing expected key %q", exp)
		}
	}
}

func TestDefaultValueForAllKeys(t *testing.T) {
	tests := []struct {
		key      string
		expected string
	}{
		{"update_interval", "24h"},
		{"github_token", ""},
		{"registry_url", ""},
		{"github_base_url", ""},
		{"policy_url", ""},
		{"config_url", ""},
		{"telemetry", "false"},
		{"require_signature", "false"},
		{"signature_public_key", ""},
		{"audit_log_destination", ""},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			got := DefaultValue(tt.key)
			if got != tt.expected {
				t.Errorf("DefaultValue(%q) = %q, want %q", tt.key, got, tt.expected)
			}
		})
	}
}

func TestFieldMapIncludesAuditLogDestination(t *testing.T) {
	cfg := Config{
		AuditLogDestination: "syslog",
	}

	fields := cfg.FieldMap()
	if fields["audit_log_destination"] != "syslog" {
		t.Errorf("FieldMap audit_log_destination = %q, want 'syslog'", fields["audit_log_destination"])
	}
}

func TestFieldMapBooleanFields(t *testing.T) {
	cfg := Config{
		Telemetry:        true,
		RequireSignature: true,
	}

	fields := cfg.FieldMap()
	if fields["telemetry"] != "true" {
		t.Errorf("FieldMap telemetry = %q, want 'true'", fields["telemetry"])
	}
	if fields["require_signature"] != "true" {
		t.Errorf("FieldMap require_signature = %q, want 'true'", fields["require_signature"])
	}

	cfg2 := Config{}
	fields2 := cfg2.FieldMap()
	if fields2["telemetry"] != "false" {
		t.Errorf("FieldMap telemetry (zero) = %q, want 'false'", fields2["telemetry"])
	}
}

func TestSetFieldAllKeys(t *testing.T) {
	tests := []struct {
		key   string
		value string
		check func(Config) bool
	}{
		{"update_interval", "6h", func(c Config) bool { return c.UpdateInterval == "6h" }},
		{"github_token", "ghp_abc", func(c Config) bool { return c.GithubToken == "ghp_abc" }},
		{"registry_url", "https://r.example.com", func(c Config) bool { return c.RegistryURL == "https://r.example.com" }},
		{"github_base_url", "https://gh.corp.com", func(c Config) bool { return c.GithubBaseURL == "https://gh.corp.com" }},
		{"policy_url", "https://p.example.com", func(c Config) bool { return c.PolicyURL == "https://p.example.com" }},
		{"config_url", "https://c.example.com", func(c Config) bool { return c.ConfigURL == "https://c.example.com" }},
		{"telemetry", "true", func(c Config) bool { return c.Telemetry }},
		{"require_signature", "yes", func(c Config) bool { return c.RequireSignature }},
		{"signature_public_key", "PEM_DATA", func(c Config) bool { return c.SignaturePublicKey == "PEM_DATA" }},
		{"audit_log_destination", "stdout", func(c Config) bool { return c.AuditLogDestination == "stdout" }},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			var cfg Config
			if err := cfg.SetField(tt.key, tt.value); err != nil {
				t.Fatalf("SetField(%q, %q) failed: %v", tt.key, tt.value, err)
			}
			if !tt.check(cfg) {
				t.Errorf("SetField(%q, %q) did not set field correctly", tt.key, tt.value)
			}
		})
	}
}

func TestSetFieldUnknownKey(t *testing.T) {
	var cfg Config
	err := cfg.SetField("nonexistent", "value")
	if err == nil {
		t.Error("expected error for unknown key, got nil")
	}
}

func TestResetField(t *testing.T) {
	cfg := Config{
		UpdateInterval:      "6h",
		AuditLogDestination: "syslog",
		Telemetry:           true,
	}

	if err := cfg.ResetField("update_interval"); err != nil {
		t.Fatal(err)
	}
	if cfg.UpdateInterval != "24h" {
		t.Errorf("expected reset to '24h', got %q", cfg.UpdateInterval)
	}

	if err := cfg.ResetField("audit_log_destination"); err != nil {
		t.Fatal(err)
	}
	if cfg.AuditLogDestination != "" {
		t.Errorf("expected reset to empty, got %q", cfg.AuditLogDestination)
	}

	if err := cfg.ResetField("telemetry"); err != nil {
		t.Fatal(err)
	}
	if cfg.Telemetry {
		t.Error("expected telemetry to be reset to false")
	}
}

func TestAuditLogDestinationSaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := Config{
		Version:             1,
		AuditLogDestination: "syslog",
	}

	if err := Save(tmpDir, cfg); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := Load(tmpDir)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if loaded.AuditLogDestination != "syslog" {
		t.Errorf("expected 'syslog' after reload, got %q", loaded.AuditLogDestination)
	}
}

func TestUpdateIntervalDuration(t *testing.T) {
	cfg := Config{UpdateInterval: "12h"}
	d := cfg.UpdateIntervalDuration()
	if d.Hours() != 12 {
		t.Errorf("expected 12h, got %v", d)
	}

	cfg2 := Config{UpdateInterval: "invalid"}
	d2 := cfg2.UpdateIntervalDuration()
	if d2.Hours() != 24 {
		t.Errorf("expected fallback 24h, got %v", d2)
	}
}
