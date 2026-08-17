package audit

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

// fakeInfo is a minimal os.FileInfo for PATH lookup tests.
type fakeInfo struct {
	mode os.FileMode
	dir  bool
}

func (f fakeInfo) Name() string       { return "" }
func (f fakeInfo) Size() int64        { return 0 }
func (f fakeInfo) Mode() os.FileMode  { return f.mode }
func (f fakeInfo) ModTime() time.Time { return time.Time{} }
func (f fakeInfo) IsDir() bool        { return f.dir }
func (f fakeInfo) Sys() any           { return nil }

// fakeEnv builds a PathEnv over an in-memory file set. links maps a path to
// its symlink-resolved target; unlisted paths resolve to themselves.
func fakeEnv(goos, pathVar, pathExt string, files map[string]fakeInfo, links map[string]string) PathEnv {
	return PathEnv{
		Path:    pathVar,
		PathExt: pathExt,
		GOOS:    goos,
		Stat: func(p string) (os.FileInfo, error) {
			if fi, ok := files[p]; ok {
				return fi, nil
			}
			return nil, errors.New("not found")
		},
		EvalSymlinks: func(p string) (string, error) {
			if t, ok := links[p]; ok {
				return t, nil
			}
			return p, nil
		},
	}
}

func presentTool(name, binaryPath string) Tool {
	return Tool{Name: name, BinaryPath: binaryPath, Present: true}
}

func TestCheckPathShadowingNotShadowed(t *testing.T) {
	binDir := "/home/u/.scuta/bin"
	env := fakeEnv("linux", binDir+":/usr/bin", "", map[string]fakeInfo{
		"/home/u/.scuta/bin/pilum": {mode: 0o755},
	}, nil)

	tools := []Tool{presentTool("pilum", "/home/u/.scuta/bin/pilum")}
	machine := CheckPathShadowing(env, tools, binDir)

	if len(machine) != 0 {
		t.Fatalf("unexpected machine findings: %v", findingCodes(machine))
	}
	if tools[0].Shadowed {
		t.Fatal("tool should not be shadowed")
	}
	if tools[0].EffectivePath != "/home/u/.scuta/bin/pilum" {
		t.Fatalf("EffectivePath = %q", tools[0].EffectivePath)
	}
	if len(tools[0].Findings) != 0 {
		t.Fatalf("unexpected findings: %v", findingCodes(tools[0].Findings))
	}
}

func TestCheckPathShadowingShadowed(t *testing.T) {
	binDir := "/home/u/.scuta/bin"
	env := fakeEnv("linux", "/attacker:"+binDir, "", map[string]fakeInfo{
		"/attacker/pilum":          {mode: 0o755},
		"/home/u/.scuta/bin/pilum": {mode: 0o755},
	}, nil)

	tools := []Tool{presentTool("pilum", "/home/u/.scuta/bin/pilum")}
	CheckPathShadowing(env, tools, binDir)

	if !tools[0].Shadowed {
		t.Fatal("tool should be shadowed")
	}
	if tools[0].EffectivePath != "/attacker/pilum" {
		t.Fatalf("EffectivePath = %q", tools[0].EffectivePath)
	}
	if len(tools[0].Findings) != 1 || tools[0].Findings[0].Code != CodeShadowedBinary {
		t.Fatalf("findings = %v", findingCodes(tools[0].Findings))
	}
	if tools[0].Findings[0].Severity != SeverityCritical {
		t.Fatalf("severity = %s", tools[0].Findings[0].Severity)
	}
	if !strings.Contains(tools[0].Findings[0].Message, "/attacker/pilum") {
		t.Fatalf("message should name the shadowing path: %s", tools[0].Findings[0].Message)
	}
}

func TestCheckPathShadowingSkipsNonExecutable(t *testing.T) {
	binDir := "/home/u/.scuta/bin"
	env := fakeEnv("linux", "/attacker:"+binDir, "", map[string]fakeInfo{
		"/attacker/pilum":          {mode: 0o644}, // not executable
		"/home/u/.scuta/bin/pilum": {mode: 0o755},
	}, nil)

	tools := []Tool{presentTool("pilum", "/home/u/.scuta/bin/pilum")}
	CheckPathShadowing(env, tools, binDir)

	if tools[0].Shadowed {
		t.Fatal("non-executable file must not count as shadowing")
	}
	if tools[0].EffectivePath != "/home/u/.scuta/bin/pilum" {
		t.Fatalf("EffectivePath = %q", tools[0].EffectivePath)
	}
}

func TestCheckPathShadowingSkipsDirectories(t *testing.T) {
	binDir := "/home/u/.scuta/bin"
	env := fakeEnv("linux", "/attacker:"+binDir, "", map[string]fakeInfo{
		"/attacker/pilum":          {mode: 0o755 | os.ModeDir, dir: true},
		"/home/u/.scuta/bin/pilum": {mode: 0o755},
	}, nil)

	tools := []Tool{presentTool("pilum", "/home/u/.scuta/bin/pilum")}
	CheckPathShadowing(env, tools, binDir)

	if tools[0].Shadowed {
		t.Fatal("directory must not count as shadowing")
	}
}

func TestCheckPathShadowingSymlinkToSameBinary(t *testing.T) {
	binDir := "/home/u/.scuta/bin"
	env := fakeEnv("linux", "/usr/local/bin:"+binDir, "", map[string]fakeInfo{
		"/usr/local/bin/pilum":     {mode: 0o755},
		"/home/u/.scuta/bin/pilum": {mode: 0o755},
	}, map[string]string{
		"/usr/local/bin/pilum": "/home/u/.scuta/bin/pilum",
	})

	tools := []Tool{presentTool("pilum", "/home/u/.scuta/bin/pilum")}
	CheckPathShadowing(env, tools, binDir)

	if tools[0].Shadowed {
		t.Fatal("symlink to the verified binary must not count as shadowing")
	}
}

func TestCheckPathShadowingBinDirMissingFromPath(t *testing.T) {
	env := fakeEnv("linux", "/usr/bin:/usr/local/bin", "", map[string]fakeInfo{}, nil)

	machine := CheckPathShadowing(env, nil, "/home/u/.scuta/bin")

	if len(machine) != 1 || machine[0].Code != CodeBinDirNotInPath {
		t.Fatalf("machine findings = %v", findingCodes(machine))
	}
	if machine[0].Severity != SeverityWarning {
		t.Fatalf("severity = %s", machine[0].Severity)
	}
}

func TestCheckPathShadowingSkipsMissingTools(t *testing.T) {
	env := fakeEnv("linux", "/usr/bin", "", map[string]fakeInfo{
		"/usr/bin/pilum": {mode: 0o755},
	}, nil)

	tools := []Tool{{Name: "pilum", BinaryPath: "/home/u/.scuta/bin/pilum", Present: false}}
	CheckPathShadowing(env, tools, "/usr/bin")

	if tools[0].Shadowed || tools[0].EffectivePath != "" {
		t.Fatal("missing tools must be skipped")
	}
}

func TestCheckPathShadowingWindowsPathExt(t *testing.T) {
	binDir := "C:/scuta/bin"
	env := fakeEnv("windows", "C:/attacker;"+binDir, ".COM;.EXE", map[string]fakeInfo{
		"C:/attacker/pilum.exe":  {mode: 0o666},
		"C:/scuta/bin/pilum.exe": {mode: 0o666},
	}, nil)

	tools := []Tool{presentTool("pilum", "C:/scuta/bin/pilum.exe")}
	CheckPathShadowing(env, tools, binDir)

	if !tools[0].Shadowed {
		t.Fatal("tool should be shadowed via PATHEXT lookup")
	}
	if tools[0].EffectivePath != "C:/attacker/pilum.exe" {
		t.Fatalf("EffectivePath = %q", tools[0].EffectivePath)
	}
}

func TestCheckPathShadowingWindowsCaseInsensitive(t *testing.T) {
	binDir := "C:/Scuta/Bin"
	env := fakeEnv("windows", binDir, "", map[string]fakeInfo{
		"C:/Scuta/Bin/pilum.exe": {mode: 0o666},
	}, nil)

	// Recorded path differs only in case from what PATH resolves to.
	tools := []Tool{presentTool("pilum", "c:/scuta/bin/PILUM.EXE")}
	machine := CheckPathShadowing(env, tools, "c:/scuta/bin")

	if len(machine) != 0 {
		t.Fatalf("bin dir comparison should be case-insensitive: %v", findingCodes(machine))
	}
	if tools[0].Shadowed {
		t.Fatal("case-differing paths must compare equal on windows")
	}
}

func TestCheckPathShadowingWindowsBareNameNotMatched(t *testing.T) {
	binDir := "C:/scuta/bin"
	env := fakeEnv("windows", "C:/attacker;"+binDir, ".EXE", map[string]fakeInfo{
		"C:/attacker/pilum":      {mode: 0o666}, // no extension: not executable on windows
		"C:/scuta/bin/pilum.exe": {mode: 0o666},
	}, nil)

	tools := []Tool{presentTool("pilum", "C:/scuta/bin/pilum.exe")}
	CheckPathShadowing(env, tools, binDir)

	if tools[0].Shadowed {
		t.Fatal("extension-less file must not match on windows")
	}
	if tools[0].EffectivePath != "C:/scuta/bin/pilum.exe" {
		t.Fatalf("EffectivePath = %q", tools[0].EffectivePath)
	}
}
