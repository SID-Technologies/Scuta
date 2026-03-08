package cmd

import (
	"github.com/sid-technologies/scuta/lib/output"

	"github.com/spf13/cobra"
)

var installCmd = &cobra.Command{
	Use:   "install <tool>",
	Short: "Install a tool from the registry",
	Long: `Downloads the correct binary for your OS/architecture from the tool's
GitHub Releases, verifies checksum, and places it in ~/.scuta/bin/.`,
	Args: cobra.MaximumNArgs(1),
	Run: func(_ *cobra.Command, _ []string) {
		output.Warning("scuta install is not yet implemented")
	},
}

//nolint:gochecknoinits // Standard Cobra pattern
func init() {
	installCmd.Flags().Bool("all", false, "Install all tools from registry")
	installCmd.Flags().String("version", "", "Install a specific version (default: latest)")
	installCmd.Flags().Bool("force", false, "Reinstall even if already installed")
	rootCmd.AddCommand(installCmd)
}
