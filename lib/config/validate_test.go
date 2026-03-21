package config

import (
	"testing"
)

func TestValidateURL(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		value   string
		wantErr bool
	}{
		// Valid HTTPS URLs
		{"valid https", "github_base_url", "https://github.example.com/api/v3", false},
		{"valid https with path", "policy_url", "https://example.com/policy.yaml", false},
		{"valid registry url", "registry_url", "https://registry.example.com/tools.yaml", false},

		// Empty values (allowed for optional fields)
		{"empty github_base_url", "github_base_url", "", false},
		{"empty policy_url", "policy_url", "", false},

		// registry_url=local (special case)
		{"registry local", "registry_url", "local", false},

		// HTTP rejection
		{"http github_base_url", "github_base_url", "http://github.example.com/api/v3", true},
		{"http policy_url", "policy_url", "http://example.com/policy.yaml", true},
		{"http registry_url", "registry_url", "http://registry.example.com/tools.yaml", true},

		// file:// rejection
		{"file scheme", "github_base_url", "file:///etc/passwd", true},

		// ftp:// rejection
		{"ftp scheme", "policy_url", "ftp://example.com/file", true},

		// Loopback rejection
		{"loopback ipv4", "github_base_url", "https://127.0.0.1/api/v3", true},
		{"loopback ipv4 other", "github_base_url", "https://127.0.0.2/api/v3", true},
		{"loopback ipv6", "github_base_url", "https://[::1]/api/v3", true},
		{"localhost", "github_base_url", "https://localhost/api/v3", true},
		{"localhost uppercase", "github_base_url", "https://LOCALHOST/api/v3", true},

		// Private IP rejection
		{"private 10.x", "policy_url", "https://10.0.0.1/policy.yaml", true},
		{"private 172.16.x", "policy_url", "https://172.16.0.1/policy.yaml", true},
		{"private 192.168.x", "policy_url", "https://192.168.1.1/policy.yaml", true},
		{"link-local", "policy_url", "https://169.254.1.1/policy.yaml", true},

		// Missing host
		{"missing host", "github_base_url", "https:///path", true},

		// update_interval (existing validation)
		{"valid duration", "update_interval", "24h", false},
		{"invalid duration", "update_interval", "not-a-duration", true},

		// github_token (no validation)
		{"github token", "github_token", "ghp_abc123", false},

		// audit_log_destination
		{"audit empty", "audit_log_destination", "", false},
		{"audit stdout", "audit_log_destination", "stdout", false},
		{"audit syslog", "audit_log_destination", "syslog", false},
		{"audit valid webhook", "audit_log_destination", "https://hooks.example.com/audit", false},
		{"audit plain string", "audit_log_destination", "not-a-url", true},
		{"audit http webhook", "audit_log_destination", "http://hooks.example.com/audit", true},
		{"audit localhost webhook", "audit_log_destination", "https://localhost/audit", true},

		// telemetry and require_signature booleans
		{"telemetry true", "telemetry", "true", false},
		{"telemetry false", "telemetry", "false", false},
		{"telemetry yes", "telemetry", "yes", false},
		{"telemetry invalid", "telemetry", "maybe", true},
		{"require_sig true", "require_signature", "true", false},
		{"require_sig invalid", "require_signature", "nope", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateValue(tt.key, tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateValue(%q, %q) error = %v, wantErr %v", tt.key, tt.value, err, tt.wantErr)
			}
		})
	}
}

func TestMaskValue(t *testing.T) {
	if got := MaskValue("github_token", "secret"); got != "****" {
		t.Errorf("expected masked value, got %q", got)
	}
	if got := MaskValue("github_token", ""); got != "" {
		t.Errorf("expected empty for empty token, got %q", got)
	}
	if got := MaskValue("registry_url", "https://example.com"); got != "https://example.com" {
		t.Errorf("expected unmasked value, got %q", got)
	}
}
