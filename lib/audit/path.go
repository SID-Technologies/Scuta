package audit

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// defaultPathExt mirrors the Windows default when PATHEXT is unset.
const defaultPathExt = ".COM;.EXE;.BAT;.CMD"

// PathEnv abstracts the pieces of the OS that PATH-shadowing checks touch,
// so tests can exercise both platforms from any host.
type PathEnv struct {
	Path         string // value of the PATH variable
	PathExt      string // value of PATHEXT (Windows executable extensions)
	GOOS         string
	Stat         func(string) (os.FileInfo, error)
	EvalSymlinks func(string) (string, error)
}

// SystemPathEnv returns a PathEnv backed by the real OS.
func SystemPathEnv() PathEnv {
	return PathEnv{
		Path:         os.Getenv("PATH"),
		PathExt:      os.Getenv("PATHEXT"),
		GOOS:         runtime.GOOS,
		Stat:         os.Stat,
		EvalSymlinks: filepath.EvalSymlinks,
	}
}

// CheckPathShadowing verifies that each present tool resolves on PATH to the
// binary scuta installed and verified. A different binary earlier on PATH is
// what actually executes, so shadowing is critical. Per-tool findings are
// attached in place; the returned findings are machine-level (scuta's bin
// directory missing from PATH entirely).
func CheckPathShadowing(env PathEnv, tools []Tool, binDir string) []Finding {
	var machine []Finding

	dirs := splitPathList(env)
	if binDir != "" && !containsDir(env, dirs, binDir) {
		machine = append(machine, Finding{
			Severity: SeverityWarning,
			Code:     CodeBinDirNotInPath,
			Message:  fmt.Sprintf("scuta bin directory %s is not on PATH — installed tools are not runnable by name", binDir),
		})
	}

	for i := range tools {
		t := &tools[i]
		if !t.Present || t.BinaryPath == "" {
			continue
		}

		effective, ok := lookPath(env, dirs, t.Name)
		if !ok {
			// Not resolvable by name at all; the bin-dir finding above
			// already explains why.
			continue
		}
		t.EffectivePath = effective

		if samePath(env, effective, t.BinaryPath) {
			continue
		}

		t.Shadowed = true
		t.Findings = append(t.Findings, Finding{
			Severity: SeverityCritical,
			Code:     CodeShadowedBinary,
			Message: fmt.Sprintf("%s resolves to %s on PATH, not the verified binary at %s — the verified binary is not what executes",
				t.Name, effective, t.BinaryPath),
		})
	}

	return machine
}

// splitPathList splits the PATH variable for the injected platform. Empty
// entries (which POSIX treats as the current directory) are skipped rather
// than guessed at.
func splitPathList(env PathEnv) []string {
	sep := ":"
	if env.GOOS == "windows" {
		sep = ";"
	}

	var dirs []string
	for _, d := range strings.Split(env.Path, sep) {
		if d == "" {
			continue
		}
		dirs = append(dirs, d)
	}

	return dirs
}

// containsDir reports whether target appears in dirs, comparing with the
// same path semantics as samePath.
func containsDir(env PathEnv, dirs []string, target string) bool {
	for _, d := range dirs {
		if samePath(env, d, target) {
			return true
		}
	}
	return false
}

// candidates returns the file names a shell would consider executable for a
// command name: the bare name on POSIX, name+PATHEXT extensions on Windows.
func candidates(env PathEnv, name string) []string {
	if env.GOOS != "windows" {
		return []string{name}
	}

	pathExt := env.PathExt
	if pathExt == "" {
		pathExt = defaultPathExt
	}

	var out []string
	for _, ext := range strings.Split(pathExt, ";") {
		if ext == "" {
			continue
		}
		out = append(out, name+strings.ToLower(ext))
	}

	return out
}

// lookPath finds the first executable on PATH that would run for name,
// mirroring exec.LookPath semantics for the injected platform.
func lookPath(env PathEnv, dirs []string, name string) (string, bool) {
	cands := candidates(env, name)
	for _, dir := range dirs {
		for _, cand := range cands {
			p := joinPath(env, dir, cand)

			info, err := env.Stat(p)
			if err != nil || info.IsDir() {
				continue
			}
			if env.GOOS != "windows" && info.Mode()&0o111 == 0 {
				continue
			}

			return p, true
		}
	}

	return "", false
}

// joinPath joins a PATH directory and a file name with a forward slash,
// which every supported platform accepts. filepath.Join would use the host
// separator, making lookups (and their cross-platform tests) depend on the
// machine running the check rather than the injected GOOS.
func joinPath(env PathEnv, dir, name string) string {
	cut := "/"
	if env.GOOS == "windows" {
		cut = `/\`
	}
	return strings.TrimRight(dir, cut) + "/" + name
}

// samePath reports whether two paths refer to the same file, resolving
// symlinks when possible and comparing case-insensitively on Windows.
func samePath(env PathEnv, a, b string) bool {
	ra := resolvePath(env, a)
	rb := resolvePath(env, b)
	if env.GOOS == "windows" {
		return strings.EqualFold(ra, rb)
	}
	return ra == rb
}

// resolvePath resolves symlinks, falling back to a lexical clean when the
// path cannot be resolved (for example, it does not exist).
func resolvePath(env PathEnv, p string) string {
	if env.EvalSymlinks != nil {
		if r, err := env.EvalSymlinks(p); err == nil {
			return filepath.Clean(r)
		}
	}
	return filepath.Clean(p)
}
