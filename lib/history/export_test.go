package history

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testEntries() []Entry {
	return []Entry{
		{
			ID:        "abc123",
			Timestamp: time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC),
			Command:   "install",
			Success:   true,
			Duration:  "1.5s",
			Tools: []ToolResult{
				{Name: "pilum", Action: "install", Version: "1.0.0", Success: true, Duration: "1.5s"},
			},
		},
		{
			ID:        "def456",
			Timestamp: time.Date(2026, 3, 2, 12, 0, 0, 0, time.UTC),
			Command:   "update",
			Success:   false,
			Duration:  "2.0s",
			Tools: []ToolResult{
				{Name: "api-gen", Action: "update", Version: "2.0.0", Success: false, Duration: "2.0s", Error: "timeout"},
			},
		},
	}
}

func TestExportToWriterJSON(t *testing.T) {
	var buf bytes.Buffer
	entries := testEntries()

	if err := ExportToWriter(&buf, entries, FormatJSON); err != nil {
		t.Fatalf("ExportToWriter(JSON) failed: %v", err)
	}

	// Parse back
	var parsed []Entry
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("parsing exported JSON: %v", err)
	}

	if len(parsed) != 2 {
		t.Errorf("expected 2 entries, got %d", len(parsed))
	}

	if parsed[0].ID != "abc123" {
		t.Errorf("expected first entry ID 'abc123', got %q", parsed[0].ID)
	}
}

func TestExportToWriterJSONL(t *testing.T) {
	var buf bytes.Buffer
	entries := testEntries()

	if err := ExportToWriter(&buf, entries, FormatJSONL); err != nil {
		t.Fatalf("ExportToWriter(JSONL) failed: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 {
		t.Errorf("expected 2 lines, got %d", len(lines))
	}

	// Parse each line
	for i, line := range lines {
		var entry Entry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("parsing line %d: %v", i, err)
		}
	}
}

func TestExportToFile(t *testing.T) {
	tmpDir := t.TempDir()
	fp := filepath.Join(tmpDir, "audit.json")

	if err := ExportToFile(testEntries(), fp, FormatJSON); err != nil {
		t.Fatalf("ExportToFile failed: %v", err)
	}

	data, err := os.ReadFile(fp)
	if err != nil {
		t.Fatalf("reading export file: %v", err)
	}

	var parsed []Entry
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("parsing exported file: %v", err)
	}

	if len(parsed) != 2 {
		t.Errorf("expected 2 entries, got %d", len(parsed))
	}
}

func TestExportToWebhook(t *testing.T) {
	var received []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received = make([]byte, r.ContentLength)
		_, _ = r.Body.Read(received)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	if err := ExportToWebhook(testEntries(), server.URL); err != nil {
		t.Fatalf("ExportToWebhook failed: %v", err)
	}

	if len(received) == 0 {
		t.Error("expected webhook to receive data")
	}
}

func TestExportToWebhookClientError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	err := ExportToWebhook(testEntries(), server.URL)
	if err == nil {
		t.Error("expected error for 400 response, got nil")
	}
}

func TestExportUnknownFormat(t *testing.T) {
	var buf bytes.Buffer
	err := ExportToWriter(&buf, testEntries(), "xml")
	if err == nil {
		t.Error("expected error for unknown format, got nil")
	}
}
