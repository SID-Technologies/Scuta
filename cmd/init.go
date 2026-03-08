package cmd

import (
	"github.com/sid-technologies/scuta/lib/output"

	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Setup Scuta on a new machine",
	Long: `Creates ~/.scuta/ directory structure, detects GitHub authentication
(gh CLI or token), adds ~/.scuta/bin/ to PATH, and prints next steps.

Idempotent — safe to run multiple times.`,
	Run: func(_ *cobra.Command, _ []string) {
		output.Warning("scuta init is not yet implemented")
	},
}

//nolint:gochecknoinits // Standard Cobra pattern
func init() {
	rootCmd.AddCommand(initCmd)
}
