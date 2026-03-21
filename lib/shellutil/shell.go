package shellutil

import (
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/sid-technologies/scuta/lib/output"
)

// IsInPath checks if the given directory is in the system PATH.
func IsInPath(dir string) bool {
	pathEnv := os.Getenv("PATH")
	for _, p := range strings.Split(pathEnv, string(os.PathListSeparator)) {
		// On Windows, paths are case-insensitive
		if runtime.GOOS == "windows" {
			if strings.EqualFold(p, dir) {
				return true
			}
		} else if p == dir {
			return true
		}
	}
	return false
}

// DetectShell returns the current shell name.
func DetectShell() string {
	// On Windows, check for PowerShell first
	if runtime.GOOS == "windows" {
		// PSModulePath is set in all PowerShell sessions
		if os.Getenv("PSModulePath") != "" {
			return "powershell"
		}
		// ComSpec typically points to cmd.exe
		comSpec := os.Getenv("ComSpec")
		if strings.Contains(strings.ToLower(comSpec), "cmd.exe") {
			return "cmd"
		}
		return "cmd"
	}

	shell := os.Getenv("SHELL")
	if strings.Contains(shell, "zsh") {
		return "zsh"
	}
	if strings.Contains(shell, "bash") {
		return "bash"
	}
	if strings.Contains(shell, "fish") {
		return "fish"
	}
	return "sh"
}

// PrintPathInstructions prints shell-specific PATH setup instructions.
func PrintPathInstructions(binDir string, shell string) {
	fmt.Println()
	switch shell {
	case "zsh":
		output.Info("Add to ~/.zshrc:")
		fmt.Printf("  export PATH=\"%s:$PATH\"\n", binDir)
	case "bash":
		output.Info("Add to ~/.bashrc:")
		fmt.Printf("  export PATH=\"%s:$PATH\"\n", binDir)
	case "fish":
		output.Info("Add to ~/.config/fish/config.fish:")
		fmt.Printf("  set -gx PATH %s $PATH\n", binDir)
	case "powershell":
		output.Info("Add to your PowerShell profile ($PROFILE):")
		fmt.Printf("  $env:Path = \"%s;\" + $env:Path\n", binDir)
		output.Dimmed("Or set permanently:")
		fmt.Printf("  [Environment]::SetEnvironmentVariable(\"Path\", \"%s;\" + [Environment]::GetEnvironmentVariable(\"Path\", \"User\"), \"User\")\n", binDir)
	case "cmd":
		output.Info("Run this in Command Prompt (as administrator):")
		fmt.Printf("  setx PATH \"%s;%%PATH%%\"\n", binDir)
	default:
		output.Info("Add to your shell profile:")
		fmt.Printf("  export PATH=\"%s:$PATH\"\n", binDir)
	}
	fmt.Println()
}
