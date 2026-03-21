//go:build !windows

package history

import (
	"testing"
	"time"
)

func TestExportToSyslogBasic(t *testing.T) {
	entries := []Entry{
		{
			ID:        "test001",
			Timestamp: time.Date(2026, 3, 19, 10, 0, 0, 0, time.UTC),
			Command:   "install",
			Success:   true,
			Duration:  "1.5s",
			Tools: []ToolResult{
				{Name: "pilum", Action: "install", Version: "1.0.0", Success: true, Duration: "1.5s"},
			},
		},
	}

	// This connects to the local syslog daemon. It should succeed on macOS/Linux.
	err := ExportToSyslog(entries, "scuta-test")
	if err != nil {
		t.Fatalf("ExportToSyslog failed: %v", err)
	}
}

func TestExportToSyslogDefaultTag(t *testing.T) {
	entries := []Entry{
		{
			ID:        "test002",
			Timestamp: time.Now(),
			Command:   "update",
			Success:   true,
			Duration:  "0.5s",
		},
	}

	// Empty tag should default to "scuta"
	err := ExportToSyslog(entries, "")
	if err != nil {
		t.Fatalf("ExportToSyslog with empty tag failed: %v", err)
	}
}

func TestExportToSyslogEmpty(t *testing.T) {
	// Exporting zero entries should not error
	err := ExportToSyslog(nil, "scuta-test")
	if err != nil {
		t.Fatalf("ExportToSyslog with nil entries failed: %v", err)
	}
}
