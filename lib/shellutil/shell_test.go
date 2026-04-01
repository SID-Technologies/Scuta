package shellutil

import (
	"os"
	"runtime"
	"testing"
)

func TestIsInPath_Found(t *testing.T) {
	// /usr/bin should always be in PATH
	if runtime.GOOS == "windows" {
		t.Skip("skipping on Windows")
	}
	if !IsInPath("/usr/bin") {
		t.Error("expected /usr/bin to be in PATH")
	}
}

func TestIsInPath_NotFound(t *testing.T) {
	if IsInPath("/nonexistent/directory/that/does/not/exist") {
		t.Error("expected nonexistent directory to not be in PATH")
	}
}

func TestIsInPath_Empty(t *testing.T) {
	if IsInPath("") {
		t.Error("expected empty string to not be in PATH")
	}
}

func TestDetectShell_Zsh(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on Windows")
	}
	t.Setenv("SHELL", "/bin/zsh")
	if got := DetectShell(); got != "zsh" {
		t.Errorf("expected 'zsh', got %q", got)
	}
}

func TestDetectShell_Bash(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on Windows")
	}
	t.Setenv("SHELL", "/bin/bash")
	if got := DetectShell(); got != "bash" {
		t.Errorf("expected 'bash', got %q", got)
	}
}

func TestDetectShell_Fish(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on Windows")
	}
	t.Setenv("SHELL", "/usr/local/bin/fish")
	if got := DetectShell(); got != "fish" {
		t.Errorf("expected 'fish', got %q", got)
	}
}

func TestDetectShell_Fallback(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on Windows")
	}
	t.Setenv("SHELL", "/bin/csh")
	if got := DetectShell(); got != "sh" {
		t.Errorf("expected 'sh' fallback, got %q", got)
	}
}

func TestDetectShell_Empty(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on Windows")
	}
	t.Setenv("SHELL", "")
	os.Unsetenv("SHELL")
	if got := DetectShell(); got != "sh" {
		t.Errorf("expected 'sh' fallback for empty SHELL, got %q", got)
	}
}
