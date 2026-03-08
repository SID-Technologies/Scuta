package cmd

import (
	"github.com/sid-technologies/scuta/lib/output"

	"github.com/spf13/cobra"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Health check — diagnose common issues",
	Long: `Checks:
  - ~/.scuta/bin/ exists and is in PATH
  - All installed binaries are executable
  - State file is valid
  - GitHub authentication is configured
  - Registry is reachable`,
	Run: func(cmd *cobra.Command, args []string) {
		output.Warning("scuta doctor is not yet implemented")
	},
}

//nolint:gochecknoinits // Standard Cobra pattern
func init() {
	rootCmd.AddCommand(doctorCmd)
}
