package path

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestEnsureDirPermissions(t *testing.T) {
	// Override home directory so EnsureDir creates dirs in a temp location
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", tmpDir)
	defer func() { os.Setenv("HOME", origHome) }()

	dir, err := EnsureDir()
	if err != nil {
		t.Fatalf("EnsureDir() failed: %v", err)
	}

	// Check scuta dir permissions
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat scuta dir: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Errorf("scuta dir permissions = %o, want 0700", perm)
	}

	// Check bin dir permissions
	binDir := filepath.Join(dir, "bin")
	binInfo, err := os.Stat(binDir)
	if err != nil {
		t.Fatalf("stat bin dir: %v", err)
	}
	if perm := binInfo.Mode().Perm(); perm != 0o700 {
		t.Errorf("bin dir permissions = %o, want 0700", perm)
	}
}

func TestSystemBinDir(t *testing.T) {
	dir := SystemBinDir()
	if dir == "" {
		t.Error("expected non-empty system bin dir")
	}

	if runtime.GOOS == "windows" {
		if !strings.Contains(dir, "Scuta") {
			t.Errorf("expected Windows path to contain 'Scuta', got %q", dir)
		}
	} else {
		if dir != "/usr/local/bin" {
			t.Errorf("expected /usr/local/bin on Unix, got %q", dir)
		}
	}
}

func TestSystemStateDir(t *testing.T) {
	dir := SystemStateDir()
	if dir == "" {
		t.Error("expected non-empty system state dir")
	}

	if runtime.GOOS == "windows" {
		if !strings.Contains(dir, "Scuta") {
			t.Errorf("expected Windows path to contain 'Scuta', got %q", dir)
		}
	} else {
		if dir != "/etc/scuta" {
			t.Errorf("expected /etc/scuta on Unix, got %q", dir)
		}
	}
}

func TestSystemStatePath(t *testing.T) {
	p := SystemStatePath()
	if p == "" {
		t.Error("expected non-empty system state path")
	}

	if !strings.HasSuffix(p, "state.json") {
		t.Errorf("expected path to end with state.json, got %q", p)
	}
}
