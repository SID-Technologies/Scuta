package cmd

import (
	"github.com/sid-technologies/scuta/lib/output"

	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "Show all tools with install status",
	Long: `Displays all tools from the registry with their current install status,
installed version, and latest available version.`,
	Run: func(_ *cobra.Command, _ []string) {
		output.Warning("scuta list is not yet implemented")
	},
}

//nolint:gochecknoinits // Standard Cobra pattern
func init() {
	rootCmd.AddCommand(listCmd)
}
