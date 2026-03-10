package path

import (
	"os"
	"path/filepath"
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
