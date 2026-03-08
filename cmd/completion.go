package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var completionCmd = &cobra.Command{
	Use:   "completion <shell>",
	Short: "Generate shell completion scripts",
	Long: `Generate shell completion scripts for bash, zsh, or fish.

To install completions:

  bash:
    scuta completion bash > /etc/bash_completion.d/scuta

  zsh:
    scuta completion zsh > "${fpath[1]}/_scuta"

  fish:
    scuta completion fish > ~/.config/fish/completions/scuta.fish`,
	Args:      cobra.ExactArgs(1),
	ValidArgs: []string{"bash", "zsh", "fish"},
	RunE:      runCompletion,
}

//nolint:gochecknoinits // Standard Cobra pattern
func init() {
	rootCmd.AddCommand(completionCmd)
}

func runCompletion(_ *cobra.Command, args []string) error {
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
}
