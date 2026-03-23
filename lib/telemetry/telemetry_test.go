package telemetry

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRecordEnabled(t *testing.T) {
	tmpDir := t.TempDir()

	if err := Record(tmpDir, true, "install"); err != nil {
		t.Fatalf("Record failed: %v", err)
	}

	// Verify file was created
	fp := filepath.Join(tmpDir, telemetryFile)
	data, err := os.ReadFile(fp)
	if err != nil {
		t.Fatalf("reading telemetry file: %v", err)
	}

	// Parse the event
	var event Event
	if err := json.Unmarshal(data[:len(data)-1], &event); err != nil { // -1 to strip trailing newline
		t.Fatalf("parsing event: %v", err)
	}

	if event.Event != "install" {
		t.Errorf("expected event 'install', got %q", event.Event)
	}
	if event.OS == "" {
		t.Error("expected OS to be set")
	}
	if event.Arch == "" {
		t.Error("expected Arch to be set")
	}
	if event.Timestamp.IsZero() {
		t.Error("expected non-zero timestamp")
	}
}

func TestRecordDisabled(t *testing.T) {
	tmpDir := t.TempDir()

	if err := Record(tmpDir, false, "install"); err != nil {
		t.Fatalf("Record failed: %v", err)
	}

	// Verify no file was created
	fp := filepath.Join(tmpDir, telemetryFile)
	if _, err := os.Stat(fp); !os.IsNotExist(err) {
		t.Error("expected no telemetry file when disabled")
	}
}

func TestRecordMultiple(t *testing.T) {
	tmpDir := t.TempDir()

	events := []string{"install", "update", "uninstall", "self-update"}
	for _, e := range events {
		if err := Record(tmpDir, true, e); err != nil {
			t.Fatalf("Record(%s) failed: %v", e, err)
		}
	}

	loaded, err := Load(tmpDir)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if len(loaded) != len(events) {
		t.Errorf("expected %d events, got %d", len(events), len(loaded))
	}

	for i, e := range loaded {
		if e.Event != events[i] {
			t.Errorf("event %d: expected %q, got %q", i, events[i], e.Event)
		}
	}
}

func TestLoadEmpty(t *testing.T) {
	tmpDir := t.TempDir()

	events, err := Load(tmpDir)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if events != nil {
		t.Errorf("expected nil events, got %d", len(events))
	}
}

func TestEnabledMessage(t *testing.T) {
	msg := EnabledMessage()
	if msg == "" {
		t.Error("expected non-empty enabled message")
	}
}
