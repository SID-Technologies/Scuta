package managers

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/sid-technologies/scuta/lib/audit"
)

// symlinkEntry is a fake DirEntry that reports as a symlinked directory.
type symlinkEntry struct{ name string }

func (s symlinkEntry) Name() string               { return s.name }
func (s symlinkEntry) IsDir() bool                { return true }
func (s symlinkEntry) Type() fs.FileMode          { return fs.ModeSymlink }
func (s symlinkEntry) Info() (fs.FileInfo, error) { return nil, errors.New("not implemented") }

func fakeMise(env map[string]string, home string, dirs map[string][]os.DirEntry) *Mise {
	return &Mise{
		getenv:      func(k string) string { return env[k] },
		userHomeDir: func() (string, error) { return home, nil },
		readDir: func(p string) ([]os.DirEntry, error) {
			if e, ok := dirs[p]; ok {
				return e, nil
			}
			return nil, errors.New("no such dir")
		},
	}
}

func TestMiseInstallsDirPrecedence(t *testing.T) {
	m := fakeMise(map[string]string{"MISE_DATA_DIR": "/data"}, "/home/u", nil)
	if got := m.installsDir(); got != filepath.Join("/data", "installs") {
		t.Fatalf("MISE_DATA_DIR should win, got %q", got)
	}

	m = fakeMise(map[string]string{"XDG_DATA_HOME": "/xdg"}, "/home/u", nil)
	if got := m.installsDir(); got != filepath.Join("/xdg", "mise", "installs") {
		t.Fatalf("XDG expected, got %q", got)
	}

	m = fakeMise(map[string]string{}, "/home/u", nil)
	want := filepath.Join("/home/u", ".local", "share", "mise", "installs")
	if got := m.installsDir(); got != want {
		t.Fatalf("home default expected, got %q", got)
	}
}

func TestMiseAuditSkipsSymlinkedPartialVersions(t *testing.T) {
	installs := filepath.Join("/data", "installs")
	m := fakeMise(map[string]string{"MISE_DATA_DIR": "/data"}, "",
		map[string][]os.DirEntry{
			installs: {fakeEntry{name: "node", dir: true}},
			filepath.Join(installs, "node"): {
				symlinkEntry{name: "18"},
				symlinkEntry{name: "18.20"},
				fakeEntry{name: "18.20.2", dir: true},
			},
		})

	rep := m.Audit(context.Background())

	if len(rep.Packages) != 1 {
		t.Fatalf("packages = %+v", rep.Packages)
	}
	pkg := rep.Packages[0]
	if pkg.Name != "node" || pkg.Version != "18.20.2" {
		t.Fatalf("pkg = %+v", pkg)
	}
	if pkg.Integrity != audit.IntegrityNotVerifiable {
		t.Fatalf("integrity = %q", pkg.Integrity)
	}
}

func TestMiseDetect(t *testing.T) {
	installs := filepath.Join("/data", "installs")
	m := fakeMise(map[string]string{"MISE_DATA_DIR": "/data"}, "", map[string][]os.DirEntry{})
	if m.Detect() {
		t.Fatal("missing installs dir must not detect")
	}

	m = fakeMise(map[string]string{"MISE_DATA_DIR": "/data"}, "",
		map[string][]os.DirEntry{installs: {fakeEntry{name: "node", dir: true}}})
	if !m.Detect() {
		t.Fatal("populated installs dir must detect")
	}
}
