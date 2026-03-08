package updater

import (
	"testing"
	"time"
)

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		name      string
		installed string
		latest    string
		want      bool
	}{
		{"same version", "1.2.3", "1.2.3", false},
		{"patch update", "1.2.3", "1.2.4", true},
		{"minor update", "1.2.3", "1.3.0", true},
		{"major update", "1.2.3", "2.0.0", true},
		{"older version", "2.0.0", "1.9.9", false},
		{"empty installed", "", "1.0.0", true},
		{"dev installed", "dev", "1.0.0", true},
		{"with v prefix", "v1.0.0", "v1.0.1", true},
		{"major only", "1", "2", true},
		{"major.minor only", "1.2", "1.3", true},
		{"pre-release same numeric", "1.0.0-rc1", "1.0.0", false},
		{"pre-release to newer", "1.0.0-rc1", "1.0.1", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CompareVersions(tt.installed, tt.latest)
			if got != tt.want {
				t.Errorf("CompareVersions(%q, %q) = %v, want %v", tt.installed, tt.latest, got, tt.want)
			}
		})
	}
}

func TestNeedsCheck(t *testing.T) {
	interval := 24 * time.Hour

	// Last check was 25 hours ago — should need check
	old := time.Now().Add(-25 * time.Hour)
	if !NeedsCheck(old, interval) {
		t.Error("expected NeedsCheck to return true for 25h old check")
	}

	// Last check was 1 hour ago — should not need check
	recent := time.Now().Add(-1 * time.Hour)
	if NeedsCheck(recent, interval) {
		t.Error("expected NeedsCheck to return false for 1h old check")
	}

	// Zero time — should need check
	if !NeedsCheck(time.Time{}, interval) {
		t.Error("expected NeedsCheck to return true for zero time")
	}
}

func TestIsHomebrew(t *testing.T) {
	// Just test that it doesn't panic
	_ = IsHomebrew()
}

func TestParseVersion(t *testing.T) {
	tests := []struct {
		input string
		want  [3]int
	}{
		{"1.2.3", [3]int{1, 2, 3}},
		{"0.1.0", [3]int{0, 1, 0}},
		{"v1.0.0", [3]int{1, 0, 0}},
		{"1", [3]int{1, 0, 0}},
		{"1.2", [3]int{1, 2, 0}},
		{"1.0.0-rc1", [3]int{1, 0, 0}},
		{"", [3]int{0, 0, 0}},
	}

	for _, tt := range tests {
		got := parseVersion(tt.input)
		if got != tt.want {
			t.Errorf("parseVersion(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}
