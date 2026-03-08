package cmd

import (
	"github.com/sid-technologies/scuta/lib/output"

	"github.com/spf13/cobra"
)

var uninstallCmd = &cobra.Command{
	Use:   "uninstall <tool>",
	Short: "Remove an installed tool",
	Long: `Removes the tool binary from ~/.scuta/bin/ and clears its state entry.
Does not affect Homebrew-installed versions of the same tool.`,
	Args: cobra.ExactArgs(1),
	Run: func(_ *cobra.Command, _ []string) {
		output.Warning("scuta uninstall is not yet implemented")
	},
}

//nolint:gochecknoinits // Standard Cobra pattern
func init() {
	rootCmd.AddCommand(uninstallCmd)
}
