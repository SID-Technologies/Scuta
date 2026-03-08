package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/sid-technologies/scuta/lib/auth"
	"github.com/sid-technologies/scuta/lib/config"
	"github.com/sid-technologies/scuta/lib/output"
	"github.com/sid-technologies/scuta/lib/path"

	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Setup Scuta on a new machine",
	Long: `Creates ~/.scuta/ directory structure, detects GitHub authentication
(gh CLI or token), adds ~/.scuta/bin/ to PATH, and prints next steps.

Idempotent — safe to run multiple times.`,
	RunE: runInit,
}

//nolint:gochecknoinits // Standard Cobra pattern
func init() {
	rootCmd.AddCommand(initCmd)
}

func runInit(_ *cobra.Command, _ []string) error {
	output.Header("Scuta Setup")

	// 1. Create directory structure
	scutaDir, err := path.EnsureDir()
	if err != nil {
		return err
	}
	output.Success("Created %s", scutaDir)

	// 2. Write default config if it doesn't exist
	configPath := scutaDir + "/config.yaml"
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		cfg := config.DefaultConfig()
		if err := config.Save(scutaDir, cfg); err != nil {
			return err
		}
		output.Success("Created default config")
	} else {
		output.Success("Config already exists")
	}

	// 3. Detect GitHub auth
	token := auth.ResolveTokenWithConfig(scutaDir)
	if token != "" {
		output.Success("GitHub authentication detected")
	} else {
		output.Warning("No GitHub token found — set SCUTA_GITHUB_TOKEN or install gh CLI")
	}

	// 4. Check if bin dir is in PATH
	binDir, err := path.BinDir()
	if err != nil {
		return err
	}

	if isInPath(binDir) {
		output.Success("%s is in PATH", binDir)
	} else {
		output.Warning("%s is not in PATH", binDir)
		shell := detectShell()
		printPathInstructions(binDir, shell)
	}

	// 5. Print next steps
	output.Header("Next Steps")
	fmt.Println("  scuta install --all    Install all available tools")
	fmt.Println("  scuta list             See available tools")
	fmt.Println("  scuta doctor           Verify everything is working")
	fmt.Println()

	shell := detectShell()
	output.Info("Shell completions: scuta completion %s", shell)
	fmt.Println()

	return nil
}

// isInPath checks if the given directory is in the system PATH.
func isInPath(dir string) bool {
	pathEnv := os.Getenv("PATH")
	for _, p := range strings.Split(pathEnv, string(os.PathListSeparator)) {
		if p == dir {
			return true
		}
	}
	return false
}

// detectShell returns the current shell name.
func detectShell() string {
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

// printPathInstructions prints shell-specific PATH setup instructions.
func printPathInstructions(binDir string, shell string) {
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
