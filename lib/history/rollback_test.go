package history

import (
	"testing"
	"time"
)

// entry builds a single-tool history Entry for tests.
func entry(tool, action, version string, success bool) Entry {
	return Entry{
		ID:        "test",
		Timestamp: time.Now(),
		Command:   action,
		Success:   success,
		Tools: []ToolResult{{
			Name:    tool,
			Action:  action,
			Version: version,
			Success: success,
		}},
	}
}

func TestPreviousVersionAfterUpdate(t *testing.T) {
	// Most recent first: update to 1.2.0, install 1.1.0.
	entries := []Entry{
		entry("fzf", "update", "1.2.0", true),
		entry("fzf", "install", "1.1.0", true),
	}

	v, ok := PreviousVersion(entries, "fzf", "1.2.0")
	if !ok || v != "1.1.0" {
		t.Fatalf("PreviousVersion = %q, %v; want 1.1.0, true", v, ok)
	}
}

func TestPreviousVersionSkipsFailedAndOtherTools(t *testing.T) {
	entries := []Entry{
		entry("fzf", "update", "1.2.0", true),
		entry("fzf", "update", "1.1.5", false), // failed — never on disk
		entry("bat", "install", "0.9.0", true), // different tool
		entry("fzf", "install", "1.1.0", true),
	}

	v, ok := PreviousVersion(entries, "fzf", "1.2.0")
	if !ok || v != "1.1.0" {
		t.Fatalf("PreviousVersion = %q, %v; want 1.1.0, true", v, ok)
	}
}

func TestPreviousVersionNormalizesVPrefix(t *testing.T) {
	entries := []Entry{
		entry("fzf", "update", "v1.2.0", true),
		entry("fzf", "install", "v1.1.0", true),
	}

	v, ok := PreviousVersion(entries, "fzf", "1.2.0")
	if !ok || v != "v1.1.0" {
		t.Fatalf("PreviousVersion = %q, %v; want v1.1.0, true", v, ok)
	}
}

func TestPreviousVersionNoHistory(t *testing.T) {
	if v, ok := PreviousVersion(nil, "fzf", "1.2.0"); ok {
		t.Fatalf("expected no previous version, got %q", v)
	}
}

func TestPreviousVersionOnlyCurrentVersion(t *testing.T) {
	entries := []Entry{
		entry("fzf", "install", "1.2.0", true),
	}

	if v, ok := PreviousVersion(entries, "fzf", "1.2.0"); ok {
		t.Fatalf("expected no previous version, got %q", v)
	}
}

func TestPreviousVersionWalksChronologyNotSemver(t *testing.T) {
	// Chronology: install 1.1.0 -> update 1.3.0 -> downgrade to 1.2.0.
	// The version on disk immediately before current was 1.3.0, so that is
	// the rollback target — history order wins, not semver ordering.
	entries := []Entry{
		entry("fzf", "install", "1.2.0", true), // established current
		entry("fzf", "update", "1.3.0", true),
		entry("fzf", "install", "1.1.0", true),
	}

	v, ok := PreviousVersion(entries, "fzf", "1.2.0")
	if !ok || v != "1.3.0" {
		t.Fatalf("PreviousVersion = %q, %v; want 1.3.0, true", v, ok)
	}
}

func TestPreviousVersionCurrentAbsentFromHistory(t *testing.T) {
	// Tool at a version history never saw (state edited, partial history):
	// fall back to the most recent differing successful version.
	entries := []Entry{
		entry("fzf", "install", "1.1.0", true),
	}

	v, ok := PreviousVersion(entries, "fzf", "9.9.9")
	if !ok || v != "1.1.0" {
		t.Fatalf("PreviousVersion = %q, %v; want 1.1.0, true", v, ok)
	}
}

func TestPreviousVersionConsecutiveRollback(t *testing.T) {
	// After rolling back 1.2.0 -> 1.1.0, the previous version of 1.1.0 is
	// 1.2.0 (documented ping-pong; use install --version to pin older).
	entries := []Entry{
		entry("fzf", "rollback", "1.1.0", true),
		entry("fzf", "update", "1.2.0", true),
		entry("fzf", "install", "1.1.0", true),
	}

	v, ok := PreviousVersion(entries, "fzf", "1.1.0")
	if !ok || v != "1.2.0" {
		t.Fatalf("PreviousVersion = %q, %v; want 1.2.0, true", v, ok)
	}
}
