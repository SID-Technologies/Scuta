package workerqueue

import (
	"testing"
)

func TestExponentialBackoffWithJitter(t *testing.T) {
	tests := []struct {
		name      string
		attempt   int
		baseDelay float64
		maxDelay  float64
	}{
		{"first attempt", 0, 1.0, 60.0},
		{"second attempt", 1, 1.0, 60.0},
		{"third attempt", 2, 1.0, 60.0},
		{"large attempt capped by max", 10, 1.0, 60.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			delay := ExponentialBackoffWithJitter(tt.attempt, tt.baseDelay, tt.maxDelay)

			// Delay should be positive
			if delay <= 0 {
				t.Errorf("ExponentialBackoffWithJitter() = %f, want positive", delay)
			}

			// Delay should not exceed 1.5x max (due to jitter)
			if delay > tt.maxDelay*1.5 {
				t.Errorf("ExponentialBackoffWithJitter() = %f, exceeds 1.5x max %f", delay, tt.maxDelay)
			}
		})
	}
}

func TestShouldRedact(t *testing.T) {
	tests := []struct {
		key      string
		expected bool
	}{
		{"GITHUB_TOKEN", true},
		{"API_KEY", true},
		{"DB_PASSWORD", true},
		{"AWS_SECRET", true},
		{"MY_CREDENTIAL", true},
		{"SCUTA_GITHUB_TOKEN", true},
		{"HOME", false},
		{"PATH", false},
		{"GOPATH", false},
		{"CI", false},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			result := ShouldRedact(tt.key)
			if result != tt.expected {
				t.Errorf("ShouldRedact(%q) = %v, want %v", tt.key, result, tt.expected)
			}
		})
	}
}

func TestRedactEnvVars(t *testing.T) {
	input := map[string]string{
		"GITHUB_TOKEN": "ghp_abc123",
		"PATH":         "/usr/bin",
		"API_KEY":      "sk-secret",
	}

	result := RedactEnvVars(input)

	if result["GITHUB_TOKEN"] != "[REDACTED]" {
		t.Errorf("GITHUB_TOKEN should be redacted, got %q", result["GITHUB_TOKEN"])
	}
	if result["API_KEY"] != "[REDACTED]" {
		t.Errorf("API_KEY should be redacted, got %q", result["API_KEY"])
	}
	if result["PATH"] != "/usr/bin" {
		t.Errorf("PATH should not be redacted, got %q", result["PATH"])
	}
}
