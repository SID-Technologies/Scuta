package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/sid-technologies/scuta/lib/exitcodes"
	"github.com/sid-technologies/scuta/lib/output"
	"github.com/sid-technologies/scuta/lib/shellutil"

	"github.com/spf13/cobra"
)

func CompletionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "completion <shell>",
		Short: "Generate shell completion scripts",
		Long: `Generate shell completion scripts for bash, zsh, or fish.

To install completions automatically:

  scuta completion install

To generate and pipe manually:

  bash:
    scuta completion bash > /etc/bash_completion.d/scuta

  zsh:
    scuta completion zsh > "${fpath[1]}/_scuta"

  fish:
    scuta completion fish > ~/.config/fish/completions/scuta.fish`,
		Args:      cobra.ExactArgs(1),
		ValidArgs: []string{"bash", "zsh", "fish"},
		RunE: func(_ *cobra.Command, args []string) error {
			switch args[0] {
			case "bash":
				return rootCmd.GenBashCompletion(os.Stdout)
			case "zsh":
				return rootCmd.GenZshCompletion(os.Stdout)
			case "fish":
				return rootCmd.GenFishCompletion(os.Stdout, true)
			default:
				return fmt.Errorf("unsupported shell %q (use bash, zsh, or fish)", args[0])
			}
		},
	}

	cmd.AddCommand(completionInstallCmd())

	return cmd
}

//nolint:gochecknoinits // Standard Cobra pattern
func init() {
	rootCmd.AddCommand(CompletionCmd())
}

func completionInstallCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install shell completions for the current shell",
		Long: `Detects the current shell and writes completion scripts to the
appropriate location. Use --shell to override auto-detection.`,
		Args: cobra.NoArgs,
		RunE: runCompletionInstall,
	}

	cmd.Flags().String("shell", "", "Shell to install completions for (bash, zsh, fish)")

	return cmd
}

func runCompletionInstall(cmd *cobra.Command, _ []string) error {
	shellFlag, _ := cmd.Flags().GetString("shell")

	shell := shellFlag
	if shell == "" {
		shell = shellutil.DetectShell()
	}

	return installCompletions(shell)
}

// installCompletions writes completion scripts for the given shell.
func installCompletions(shell string) error {
	switch shell {
	case "bash":
		return installBashCompletions()
	case "zsh":
		return installZshCompletions()
	case "fish":
		return installFishCompletions()
	default:
		return exitcodes.NewError(exitcodes.InvalidArgs, fmt.Sprintf("unsupported shell %q (use bash, zsh, or fish)", shell))
	}
}

func installBashCompletions() error {
	// Prefer user-local directory
	dir := filepath.Join(os.Getenv("HOME"), ".local", "share", "bash-completion", "completions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	path := filepath.Join(dir, "scuta")
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	if err := rootCmd.GenBashCompletion(f); err != nil {
		return err
	}

	output.Success("Bash completions installed to %s", path)
	return nil
}

func installZshCompletions() error {
	dir := filepath.Join(os.Getenv("HOME"), ".zsh", "completions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	path := filepath.Join(dir, "_scuta")
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	if err := rootCmd.GenZshCompletion(f); err != nil {
		return err
	}

	output.Success("Zsh completions installed to %s", path)
	output.Info("Ensure %s is in your fpath. Add to ~/.zshrc:", dir)
	fmt.Printf("  fpath=(%s $fpath)\n", dir)
	fmt.Println("  autoload -Uz compinit && compinit")
	return nil
}

func installFishCompletions() error {
	dir := filepath.Join(os.Getenv("HOME"), ".config", "fish", "completions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	path := filepath.Join(dir, "scuta.fish")
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	if err := rootCmd.GenFishCompletion(f, true); err != nil {
		return err
	}

	output.Success("Fish completions installed to %s", path)
	return nil
}

// completionsInstalled checks if completions are already installed for the given shell.
func completionsInstalled(shell string) bool {
	home := os.Getenv("HOME")
	var paths []string

	switch shell {
	case "bash":
		paths = []string{
			filepath.Join(home, ".local", "share", "bash-completion", "completions", "scuta"),
			"/etc/bash_completion.d/scuta",
		}
	case "zsh":
		paths = []string{
			filepath.Join(home, ".zsh", "completions", "_scuta"),
		}
	case "fish":
		paths = []string{
			filepath.Join(home, ".config", "fish", "completions", "scuta.fish"),
		}
	default:
		return false
	}

	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return true
		}
	}
	return false
}
