//go:build e2e

package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// e2e target defaults. Overridable via env so the suite can be pointed at any
// public GitHub release without code changes (and so a tool changing its
// release layout never requires a code edit to keep CI green).
//
//	SCUTA_E2E_REPO     owner/repo to install         (default junegunn/fzf)
//	SCUTA_E2E_VERSION  release tag to pin             (default 0.55.0)
//	SCUTA_E2E_BIN      installed binary / tool name   (default fzf)
//	SCUTA_E2E_SKIP     set to any value to skip
const (
	defaultRepo    = "junegunn/fzf"
	defaultVersion = "0.55.0"
	defaultBin     = "fzf"
)

// scutaBin is the path to the scuta binary built once in TestMain.
var scutaBin string

func TestMain(m *testing.M) {
	if os.Getenv("SCUTA_E2E_SKIP") != "" {
		// Nothing to build; individual tests also guard on this.
		os.Exit(m.Run())
	}

	bin, cleanup, err := buildScuta()
	if err != nil {
		// Can't use t.Fatal here; print and exit non-zero.
		os.Stderr.WriteString("e2e: failed to build scuta: " + err.Error() + "\n")
		os.Exit(1)
	}
	scutaBin = bin

	code := m.Run()
	cleanup()
	os.Exit(code)
}

// buildScuta compiles the scuta binary from the module root into a temp dir.
func buildScuta() (bin string, cleanup func(), err error) {
	repoRoot := moduleRoot()
	dir, err := os.MkdirTemp("", "scuta-e2e-bin-*")
	if err != nil {
		return "", func() {}, err
	}

	name := "scuta"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	out := filepath.Join(dir, name)

	cmd := exec.Command("go", "build", "-o", out, ".")
	cmd.Dir = repoRoot
	cmd.Env = os.Environ()
	if combined, buildErr := cmd.CombinedOutput(); buildErr != nil {
		os.RemoveAll(dir)
		return "", func() {}, wrapf(buildErr, string(combined))
	}

	return out, func() { os.RemoveAll(dir) }, nil
}

// moduleRoot resolves the Scuta module root relative to this test file.
func moduleRoot() string {
	_, thisFile, _, _ := runtime.Caller(0)
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
}

// env returns a config-string helper with a default.
func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// runScuta runs the scuta binary with an isolated HOME so it never touches the
// developer's real ~/.scuta. Returns combined output and any error.
func runScuta(t *testing.T, home string, args ...string) (string, error) {
	t.Helper()

	cmd := exec.Command(scutaBin, args...)

	// Isolate all state into the sandbox home.
	environ := []string{
		"HOME=" + home,
		"USERPROFILE=" + home, // Windows
		"PATH=" + os.Getenv("PATH"),
	}
	// Pass the GitHub token through for rate limits, mapping the conventional
	// GITHUB_TOKEN to the one scuta reads if the scuta-specific one is unset.
	if tok := os.Getenv("SCUTA_GITHUB_TOKEN"); tok != "" {
		environ = append(environ, "SCUTA_GITHUB_TOKEN="+tok)
	} else if tok := os.Getenv("GITHUB_TOKEN"); tok != "" {
		environ = append(environ, "SCUTA_GITHUB_TOKEN="+tok)
	}
	cmd.Env = environ

	out, err := cmd.CombinedOutput()
	return string(out), err
}

// wrapf builds an error carrying subprocess output for readable failures.
func wrapf(err error, output string) error {
	return &e2eError{err: err, output: output}
}

type e2eError struct {
	err    error
	output string
}

func (e *e2eError) Error() string {
	return e.err.Error() + "\n--- output ---\n" + e.output
}

// TestInstallRunUninstall exercises the real lifecycle end to end:
//
//	install (direct owner/repo) -> binary present + executable -> runs -> list ->
//	uninstall -> binary gone.
func TestInstallRunUninstall(t *testing.T) {
	if os.Getenv("SCUTA_E2E_SKIP") != "" {
		t.Skip("SCUTA_E2E_SKIP set")
	}

	repo := env("SCUTA_E2E_REPO", defaultRepo)
	version := env("SCUTA_E2E_VERSION", defaultVersion)
	bin := env("SCUTA_E2E_BIN", defaultBin)
	tool := strings.ToLower(bin)

	home := t.TempDir()

	// Init the sandbox (local-only registry keeps this hermetic apart from the
	// direct install we are actually testing).
	if out, err := runScuta(t, home, "config", "set", "registry_url", "local"); err != nil {
		t.Fatalf("config set registry_url local failed: %v\n%s", err, out)
	}

	// 1. Install directly from the public repo, pinned to a known version.
	installArgs := []string{"install", repo, "--version", version}
	out, err := runScuta(t, home, installArgs...)
	if err != nil {
		t.Fatalf("install %s@%s failed: %v\n%s", repo, version, err, out)
	}
	t.Logf("install output:\n%s", out)

	// 2. Binary must exist and be executable.
	binName := bin
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	binPath := filepath.Join(home, ".scuta", "bin", binName)
	info, statErr := os.Stat(binPath)
	if statErr != nil {
		t.Fatalf("expected installed binary at %s: %v", binPath, statErr)
	}
	if runtime.GOOS != "windows" && info.Mode()&0o111 == 0 {
		t.Fatalf("installed binary %s is not executable (mode %v)", binPath, info.Mode())
	}

	// 3. The installed binary actually runs.
	verCmd := exec.Command(binPath, "--version")
	verCmd.Env = append(os.Environ(), "HOME="+home)
	done := make(chan error, 1)
	go func() { _, e := verCmd.CombinedOutput(); done <- e }()
	select {
	case e := <-done:
		if e != nil {
			t.Fatalf("running %s --version failed: %v", binPath, e)
		}
	case <-time.After(30 * time.Second):
		_ = verCmd.Process.Kill()
		t.Fatalf("%s --version timed out", binPath)
	}

	// 4. scuta list should report the tool as installed.
	listOut, listErr := runScuta(t, home, "list")
	if listErr != nil {
		t.Fatalf("list failed: %v\n%s", listErr, listOut)
	}
	if !strings.Contains(strings.ToLower(listOut), tool) {
		t.Errorf("expected %q in list output, got:\n%s", tool, listOut)
	}

	// 5. Uninstall and confirm the binary is gone.
	uninstOut, uninstErr := runScuta(t, home, "uninstall", tool)
	if uninstErr != nil {
		t.Fatalf("uninstall %s failed: %v\n%s", tool, uninstErr, uninstOut)
	}
	if _, err := os.Stat(binPath); !os.IsNotExist(err) {
		t.Fatalf("expected binary removed at %s, stat err = %v", binPath, err)
	}
}
