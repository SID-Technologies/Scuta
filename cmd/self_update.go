package cmd

import (
	"github.com/sid-technologies/scuta/lib/output"

	"github.com/spf13/cobra"
)

var selfUpdateCmd = &cobra.Command{
	Use:   "self-update",
	Short: "Update Scuta itself",
	Long: `Downloads the latest Scuta release and replaces the current binary.
If installed via Homebrew, prints guidance to use brew upgrade instead.`,
	Run: func(_ *cobra.Command, _ []string) {
		output.Warning("scuta self-update is not yet implemented")
	},
}

//nolint:gochecknoinits // Standard Cobra pattern
func init() {
	rootCmd.AddCommand(selfUpdateCmd)
}
