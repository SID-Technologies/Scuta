package cmd

import (
	"github.com/sid-technologies/scuta/lib/output"

	"github.com/spf13/cobra"
)

var updateCmd = &cobra.Command{
	Use:   "update [tool]",
	Short: "Update tools to their latest versions",
	Long: `With no arguments, updates ALL installed tools AND scuta itself.
With a tool name, updates only that tool.`,
	Args: cobra.MaximumNArgs(1),
	Run: func(_ *cobra.Command, _ []string) {
		output.Warning("scuta update is not yet implemented")
	},
}

//nolint:gochecknoinits // Standard Cobra pattern
func init() {
	rootCmd.AddCommand(updateCmd)
}
