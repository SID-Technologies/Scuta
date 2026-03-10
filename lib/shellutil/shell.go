package shellutil

import (
	"fmt"
	"os"
	"strings"

	"github.com/sid-technologies/scuta/lib/output"
)

// IsInPath checks if the given directory is in the system PATH.
func IsInPath(dir string) bool {
	pathEnv := os.Getenv("PATH")
	for _, p := range strings.Split(pathEnv, string(os.PathListSeparator)) {
		if p == dir {
			return true
		}
	}
	return false
}

// DetectShell returns the current shell name.
func DetectShell() string {
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
	default:
		output.Info("Add to your shell profile:")
		fmt.Printf("  export PATH=\"%s:$PATH\"\n", binDir)
	}
	fmt.Println()
}
