package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMergedToolsUserOnly(t *testing.T) {
	userState := NewState()
	userState.SetTool("pilum", ToolState{
		Version:     "1.0.0",
		InstalledAt: time.Now(),
		BinaryPath:  "/home/user/.scuta/bin/pilum",
	})
	userState.SetTool("api-gen", ToolState{
		Version:     "2.0.0",
		InstalledAt: time.Now(),
		BinaryPath:  "/home/user/.scuta/bin/api-gen",
	})

	entries := MergedTools(userState, "/nonexistent/state.json")

	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	for _, entry := range entries {
		if entry.Source != "user" {
			t.Errorf("expected source 'user', got %q for %s", entry.Source, entry.Name)
		}
	}
}

func TestMergedToolsSystemOnly(t *testing.T) {
	tmpDir := t.TempDir()
	systemStatePath := filepath.Join(tmpDir, "state.json")

	systemState := &State{
		Version: 1,
		Tools: map[string]ToolState{
			"pilum": {
				Version:     "1.0.0",
				InstalledAt: time.Now(),
				BinaryPath:  "/usr/local/bin/pilum",
			},
		},
	}

	data, err := json.MarshalIndent(systemState, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(systemStatePath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	// Nil user state
	entries := MergedTools(nil, systemStatePath)

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Name != "pilum" {
		t.Errorf("expected 'pilum', got %q", entries[0].Name)
	}
	if entries[0].Source != "system" {
		t.Errorf("expected source 'system', got %q", entries[0].Source)
	}
}

func TestMergedToolsUserPrecedence(t *testing.T) {
	tmpDir := t.TempDir()
	systemStatePath := filepath.Join(tmpDir, "state.json")

	// System has pilum 1.0.0
	systemState := &State{
		Version: 1,
		Tools: map[string]ToolState{
			"pilum": {
				Version:     "1.0.0",
				InstalledAt: time.Now(),
				BinaryPath:  "/usr/local/bin/pilum",
			},
			"system-only-tool": {
				Version:     "3.0.0",
				InstalledAt: time.Now(),
				BinaryPath:  "/usr/local/bin/system-only-tool",
			},
		},
	}

	data, err := json.MarshalIndent(systemState, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(systemStatePath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	// User has pilum 2.0.0 (should take precedence)
	userState := NewState()
	userState.SetTool("pilum", ToolState{
		Version:     "2.0.0",
		InstalledAt: time.Now(),
		BinaryPath:  "/home/user/.scuta/bin/pilum",
	})

	entries := MergedTools(userState, systemStatePath)

	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	// Build lookup
	entryMap := make(map[string]ToolEntry)
	for _, e := range entries {
		entryMap[e.Name] = e
	}

	// User version should win
	pilum := entryMap["pilum"]
	if pilum.Version != "2.0.0" {
		t.Errorf("expected user version 2.0.0, got %q", pilum.Version)
	}
	if pilum.Source != "user" {
		t.Errorf("expected source 'user', got %q", pilum.Source)
	}

	// System-only tool should still appear
	sot := entryMap["system-only-tool"]
	if sot.Version != "3.0.0" {
		t.Errorf("expected system version 3.0.0, got %q", sot.Version)
	}
	if sot.Source != "system" {
		t.Errorf("expected source 'system', got %q", sot.Source)
	}
}

func TestMergedToolsNoSystemFile(t *testing.T) {
	userState := NewState()
	userState.SetTool("pilum", ToolState{
		Version: "1.0.0",
	})

	// Non-existent system state — should gracefully return only user tools
	entries := MergedTools(userState, "/nonexistent/path/state.json")

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Source != "user" {
		t.Errorf("expected source 'user', got %q", entries[0].Source)
	}
}

func TestMergedToolsCorruptSystemFile(t *testing.T) {
	tmpDir := t.TempDir()
	systemStatePath := filepath.Join(tmpDir, "state.json")

	// Write invalid JSON
	if err := os.WriteFile(systemStatePath, []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	userState := NewState()
	userState.SetTool("pilum", ToolState{
		Version: "1.0.0",
	})

	// Should gracefully ignore corrupt system state
	entries := MergedTools(userState, systemStatePath)

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
}

func TestStateRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()

	st := NewState()
	st.SetTool("pilum", ToolState{
		Version:     "1.0.0",
		InstalledAt: time.Now().Truncate(time.Second),
		BinaryPath:  "/home/user/.scuta/bin/pilum",
	})

	if err := st.Save(tmpDir); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	loaded, err := Load(tmpDir)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	ts, ok := loaded.GetTool("pilum")
	if !ok {
		t.Fatal("expected pilum to be in loaded state")
	}
	if ts.Version != "1.0.0" {
		t.Errorf("expected version 1.0.0, got %q", ts.Version)
	}
	if ts.BinaryPath != "/home/user/.scuta/bin/pilum" {
		t.Errorf("expected binary path, got %q", ts.BinaryPath)
	}
}

func TestRemoveTool(t *testing.T) {
	st := NewState()
	st.SetTool("pilum", ToolState{Version: "1.0.0"})
	st.SetTool("api-gen", ToolState{Version: "2.0.0"})

	st.RemoveTool("pilum")

	if _, ok := st.GetTool("pilum"); ok {
		t.Error("expected pilum to be removed")
	}
	if _, ok := st.GetTool("api-gen"); !ok {
		t.Error("expected api-gen to still exist")
	}
}
