// Package telemetry provides opt-in, anonymous usage tracking.
// Telemetry is disabled by default. When enabled, events are recorded to a local
// JSONL file (~/.scuta/telemetry.jsonl). No PII is collected — only event types,
// OS/architecture, and timestamps.
package telemetry

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/sid-technologies/scuta/lib/errors"
)

const telemetryFile = "telemetry.jsonl"

// Event represents a single telemetry event.
type Event struct {
	Timestamp time.Time `json:"timestamp"`
	Event     string    `json:"event"`
	OS        string    `json:"os"`
	Arch      string    `json:"arch"`
}

// Record appends a telemetry event to the local JSONL file.
// This is a no-op if telemetry is disabled (enabled=false).
func Record(scutaDir string, enabled bool, event string) error {
	if !enabled {
		return nil
	}

	e := Event{
		Timestamp: time.Now().UTC(),
		Event:     event,
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
	}

	data, err := json.Marshal(e)
	if err != nil {
		return errors.Wrap(err, "marshaling telemetry event")
	}
	data = append(data, '\n')

	fp := filepath.Join(scutaDir, telemetryFile)
	f, err := os.OpenFile(fp, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return errors.Wrap(err, "opening telemetry file")
	}
	defer f.Close()

	if _, err := f.Write(data); err != nil {
		return errors.Wrap(err, "writing telemetry event")
	}

	return nil
}

// Load reads all telemetry events from disk.
func Load(scutaDir string) ([]Event, error) {
	fp := filepath.Join(scutaDir, telemetryFile)

	data, err := os.ReadFile(fp)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, errors.Wrap(err, "reading telemetry file")
	}

	var events []Event
	for _, line := range splitLines(data) {
		if len(line) == 0 {
			continue
		}
		var e Event
		if err := json.Unmarshal(line, &e); err != nil {
			continue // skip malformed lines
		}
		events = append(events, e)
	}

	return events, nil
}

// splitLines splits data by newlines, returning byte slices.
func splitLines(data []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i, b := range data {
		if b == '\n' {
			lines = append(lines, data[start:i])
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, data[start:])
	}
	return lines
}

// EnabledMessage returns the message to show when telemetry is first enabled.
func EnabledMessage() string {
	return `Telemetry enabled. Scuta collects anonymous usage data:
  - Event type (install, update, uninstall, self-update)
  - OS and architecture
  - Timestamp
No tool names, versions, or personal information is collected.
Data is stored locally in ~/.scuta/telemetry.jsonl.`
}
