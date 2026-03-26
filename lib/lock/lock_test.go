package lock

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAcquireRelease(t *testing.T) {
	dir := t.TempDir()

	err := Acquire(dir, "install", []string{"fzf"}, false)
	if err != nil {
		t.Fatalf("unexpected error acquiring lock: %v", err)
	}

	// Lock file should exist
	fp := FilePath(dir)
	if _, err := os.Stat(fp); os.IsNotExist(err) {
		t.Fatal("expected lock file to exist")
	}

	// Verify lock info
	data, err := os.ReadFile(fp)
	if err != nil {
		t.Fatal(err)
	}
	var info Info
	if err := json.Unmarshal(data, &info); err != nil {
		t.Fatal(err)
	}
	if info.Command != "install" {
		t.Errorf("expected command 'install', got %q", info.Command)
	}
	if len(info.Tools) != 1 || info.Tools[0] != "fzf" {
		t.Errorf("expected tools [fzf], got %v", info.Tools)
	}
	if info.PID != os.Getpid() {
		t.Errorf("expected PID %d, got %d", os.Getpid(), info.PID)
	}

	// Release
	Release(dir)
	if _, err := os.Stat(fp); !os.IsNotExist(err) {
		t.Fatal("expected lock file to be removed after release")
	}
}

func TestAcquire_AlreadyLocked(t *testing.T) {
	dir := t.TempDir()

	// Create a lock with current PID (so it's not stale)
	fp := FilePath(dir)
	hostname, _ := os.Hostname()
	info := Info{
		PID:       os.Getpid(),
		Hostname:  hostname,
		Timestamp: time.Now(),
		Tools:     []string{"bat"},
		Command:   "install",
	}
	data, _ := json.Marshal(info)
	if err := os.WriteFile(fp, data, 0o600); err != nil {
		t.Fatal(err)
	}

	// Attempt to acquire should fail
	err := Acquire(dir, "install", []string{"fzf"}, false)
	if err == nil {
		t.Fatal("expected error when lock already held")
	}
}

func TestAcquire_ForceOverride(t *testing.T) {
	dir := t.TempDir()

	// Create a lock
	fp := FilePath(dir)
	hostname, _ := os.Hostname()
	info := Info{
		PID:       os.Getpid(),
		Hostname:  hostname,
		Timestamp: time.Now(),
		Tools:     []string{"bat"},
		Command:   "install",
	}
	data, _ := json.Marshal(info)
	if err := os.WriteFile(fp, data, 0o600); err != nil {
		t.Fatal(err)
	}

	// Force acquire should succeed
	err := Acquire(dir, "update", []string{"fzf"}, true)
	if err != nil {
		t.Fatalf("expected force to override lock, got: %v", err)
	}

	Release(dir)
}

func TestAcquire_StaleLock(t *testing.T) {
	dir := t.TempDir()

	// Create a stale lock (timestamp > 1 hour ago)
	fp := FilePath(dir)
	info := Info{
		PID:       99999999, // unlikely to exist
		Hostname:  "other-host",
		Timestamp: time.Now().Add(-2 * time.Hour),
		Tools:     []string{"old-tool"},
		Command:   "install",
	}
	data, _ := json.Marshal(info)
	if err := os.WriteFile(fp, data, 0o600); err != nil {
		t.Fatal(err)
	}

	// Should succeed because the lock is stale
	err := Acquire(dir, "install", []string{"fzf"}, false)
	if err != nil {
		t.Fatalf("expected stale lock to be cleaned up, got: %v", err)
	}

	Release(dir)
}

func TestRelease_NoLock(t *testing.T) {
	dir := t.TempDir()

	// Should not panic or error
	Release(dir)
}

func TestFilePath(t *testing.T) {
	got := FilePath("/home/user/.scuta")
	expected := filepath.Join("/home/user/.scuta", "install.lock")
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}
