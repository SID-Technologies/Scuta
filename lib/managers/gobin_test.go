package managers

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime/debug"
	"testing"

	"github.com/sid-technologies/scuta/lib/audit"
)

// fakeEntry is a minimal os.DirEntry for adapter tests.
type fakeEntry struct {
	name string
	dir  bool
}

func (f fakeEntry) Name() string               { return f.name }
func (f fakeEntry) IsDir() bool                { return f.dir }
func (f fakeEntry) Type() fs.FileMode          { return 0 }
func (f fakeEntry) Info() (fs.FileInfo, error) { return nil, errors.New("not implemented") }

// fakeGoBin builds a GoBin over an in-memory GOBIN directory. infos maps
// binary path to its build info; paths not present are not Go binaries.
func fakeGoBin(env map[string]string, home string, entries []os.DirEntry, infos map[string]*debug.BuildInfo) *GoBin {
	return &GoBin{
		getenv:      func(k string) string { return env[k] },
		userHomeDir: func() (string, error) { return home, nil },
		readDir: func(dir string) ([]os.DirEntry, error) {
			if entries == nil {
				return nil, errors.New("no such dir")
			}
			return entries, nil
		},
		readBuildInfo: func(p string) (*debug.BuildInfo, error) {
			if bi, ok := infos[p]; ok {
				return bi, nil
			}
			return nil, errors.New("not a go binary")
		},
	}
}

func modInfo(path, version, sum string, settings map[string]string) *debug.BuildInfo {
	bi := &debug.BuildInfo{Main: debug.Module{Path: path, Version: version, Sum: sum}}
	for k, v := range settings {
		bi.Settings = append(bi.Settings, debug.BuildSetting{Key: k, Value: v})
	}
	return bi
}

func TestGoBinBinDirPrecedence(t *testing.T) {
	g := fakeGoBin(map[string]string{"GOBIN": "/gobin", "GOPATH": "/gopath"}, "/home/u", nil, nil)
	if got := g.binDir(); got != "/gobin" {
		t.Fatalf("GOBIN should win, got %q", got)
	}

	g = fakeGoBin(map[string]string{"GOPATH": "/gopath"}, "/home/u", nil, nil)
	if got := g.binDir(); got != filepath.Join("/gopath", "bin") {
		t.Fatalf("GOPATH/bin expected, got %q", got)
	}

	g = fakeGoBin(map[string]string{}, "/home/u", nil, nil)
	if got := g.binDir(); got != filepath.Join("/home/u", "go", "bin") {
		t.Fatalf("HOME/go/bin expected, got %q", got)
	}
}

func TestGoBinDetect(t *testing.T) {
	g := fakeGoBin(map[string]string{"GOBIN": "/gobin"}, "", nil, nil)
	if g.Detect() {
		t.Fatal("unreadable dir must not detect")
	}

	g = fakeGoBin(map[string]string{"GOBIN": "/gobin"}, "", []os.DirEntry{}, nil)
	if g.Detect() {
		t.Fatal("empty dir must not detect")
	}

	g = fakeGoBin(map[string]string{"GOBIN": "/gobin"}, "", []os.DirEntry{fakeEntry{name: "tool"}}, nil)
	if !g.Detect() {
		t.Fatal("non-empty dir must detect")
	}
}

func TestGoBinAuditRecordedIntegrity(t *testing.T) {
	bin := filepath.Join("/gobin", "gopls")
	g := fakeGoBin(map[string]string{"GOBIN": "/gobin"}, "",
		[]os.DirEntry{fakeEntry{name: "gopls"}},
		map[string]*debug.BuildInfo{bin: modInfo("golang.org/x/tools/gopls", "v0.16.2", "h1:abc123", nil)})

	rep := g.Audit(context.Background())

	if len(rep.Packages) != 1 {
		t.Fatalf("packages = %d", len(rep.Packages))
	}
	pkg := rep.Packages[0]
	if pkg.Integrity != audit.IntegrityRecorded {
		t.Fatalf("integrity = %q", pkg.Integrity)
	}
	if pkg.Source != "golang.org/x/tools/gopls" || pkg.Version != "v0.16.2" {
		t.Fatalf("source/version = %q %q", pkg.Source, pkg.Version)
	}
	if len(pkg.Findings) != 0 {
		t.Fatalf("unexpected findings: %v", pkg.Findings)
	}
}

func TestGoBinAuditDevelBuild(t *testing.T) {
	bin := filepath.Join("/gobin", "hack")
	g := fakeGoBin(map[string]string{"GOBIN": "/gobin"}, "",
		[]os.DirEntry{fakeEntry{name: "hack"}},
		map[string]*debug.BuildInfo{bin: modInfo("example.com/hack", "(devel)", "", nil)})

	pkg := g.Audit(context.Background()).Packages[0]

	if pkg.Integrity != audit.IntegrityNotVerifiable {
		t.Fatalf("integrity = %q", pkg.Integrity)
	}
	if len(pkg.Findings) != 1 || pkg.Findings[0].Code != audit.CodeUnpinnedBuild {
		t.Fatalf("findings = %+v", pkg.Findings)
	}
	if pkg.Findings[0].Severity != audit.SeverityInfo {
		t.Fatalf("severity = %s", pkg.Findings[0].Severity)
	}
}

func TestGoBinAuditMissingSum(t *testing.T) {
	bin := filepath.Join("/gobin", "tool")
	g := fakeGoBin(map[string]string{"GOBIN": "/gobin"}, "",
		[]os.DirEntry{fakeEntry{name: "tool"}},
		map[string]*debug.BuildInfo{bin: modInfo("example.com/tool", "v1.0.0", "", nil)})

	pkg := g.Audit(context.Background()).Packages[0]

	if pkg.Integrity != audit.IntegrityNotVerifiable {
		t.Fatalf("integrity = %q", pkg.Integrity)
	}
	if len(pkg.Findings) != 1 || pkg.Findings[0].Code != audit.CodeNoIntegrityData {
		t.Fatalf("findings = %+v", pkg.Findings)
	}
}

func TestGoBinAuditDirtyBuild(t *testing.T) {
	bin := filepath.Join("/gobin", "tool")
	settings := map[string]string{"vcs.modified": "true", "vcs.revision": "abcdef1234567890"}
	g := fakeGoBin(map[string]string{"GOBIN": "/gobin"}, "",
		[]os.DirEntry{fakeEntry{name: "tool"}},
		map[string]*debug.BuildInfo{bin: modInfo("example.com/tool", "v1.0.0", "h1:x", settings)})

	pkg := g.Audit(context.Background()).Packages[0]

	if pkg.Integrity != audit.IntegrityRecorded {
		t.Fatalf("integrity = %q", pkg.Integrity)
	}
	if len(pkg.Findings) != 1 || pkg.Findings[0].Code != audit.CodeDirtyBuild {
		t.Fatalf("findings = %+v", pkg.Findings)
	}
	if pkg.Findings[0].Severity != audit.SeverityWarning {
		t.Fatalf("severity = %s", pkg.Findings[0].Severity)
	}
}

func TestGoBinAuditSkipsNonGoBinaries(t *testing.T) {
	g := fakeGoBin(map[string]string{"GOBIN": "/gobin"}, "",
		[]os.DirEntry{fakeEntry{name: "python-script"}, fakeEntry{name: "subdir", dir: true}},
		map[string]*debug.BuildInfo{})

	rep := g.Audit(context.Background())

	if len(rep.Packages) != 0 {
		t.Fatalf("packages = %+v", rep.Packages)
	}
	if len(rep.Findings) != 0 {
		t.Fatalf("findings = %+v", rep.Findings)
	}
}

func TestGoBinAuditSortsByName(t *testing.T) {
	g := fakeGoBin(map[string]string{"GOBIN": "/gobin"}, "",
		[]os.DirEntry{fakeEntry{name: "zeta"}, fakeEntry{name: "alpha"}},
		map[string]*debug.BuildInfo{
			filepath.Join("/gobin", "zeta"):  modInfo("example.com/zeta", "v1.0.0", "h1:z", nil),
			filepath.Join("/gobin", "alpha"): modInfo("example.com/alpha", "v1.0.0", "h1:a", nil),
		})

	rep := g.Audit(context.Background())

	if len(rep.Packages) != 2 || rep.Packages[0].Name != "alpha" || rep.Packages[1].Name != "zeta" {
		t.Fatalf("packages = %+v", rep.Packages)
	}
}

func TestCollectListsUndetectedManagers(t *testing.T) {
	g := fakeGoBin(map[string]string{"GOBIN": "/gobin"}, "", nil, nil)

	sys := Collect(context.Background(), []Manager{g})

	if len(sys.Managers) != 1 {
		t.Fatalf("managers = %+v", sys.Managers)
	}
	if sys.Managers[0].Name != "go" || sys.Managers[0].Detected {
		t.Fatalf("undetected manager should still be listed: %+v", sys.Managers[0])
	}
}

func TestCollectRunsDetectedManagers(t *testing.T) {
	bin := filepath.Join("/gobin", "gopls")
	g := fakeGoBin(map[string]string{"GOBIN": "/gobin"}, "",
		[]os.DirEntry{fakeEntry{name: "gopls"}},
		map[string]*debug.BuildInfo{bin: modInfo("golang.org/x/tools/gopls", "v0.16.2", "h1:abc", nil)})

	sys := Collect(context.Background(), []Manager{g})

	if !sys.Managers[0].Detected || len(sys.Managers[0].Packages) != 1 {
		t.Fatalf("managers = %+v", sys.Managers)
	}
}
