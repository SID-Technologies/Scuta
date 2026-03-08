package cmd

import (
	"os"

	"github.com/sid-technologies/scuta/lib/auth"
	"github.com/sid-technologies/scuta/lib/github"
	"github.com/sid-technologies/scuta/lib/installer"
	"github.com/sid-technologies/scuta/lib/output"
	"github.com/sid-technologies/scuta/lib/path"
	"github.com/sid-technologies/scuta/lib/updater"

	"github.com/spf13/cobra"
)

var selfUpdateCmd = &cobra.Command{
	Use:   "self-update",
	Short: "Update Scuta itself",
	Long: `Downloads the latest Scuta release and replaces the current binary.
If installed via Homebrew, prints guidance to use brew upgrade instead.`,
	RunE: runSelfUpdate,
}

//nolint:gochecknoinits // Standard Cobra pattern
func init() {
	rootCmd.AddCommand(selfUpdateCmd)
}

func runSelfUpdate(_ *cobra.Command, _ []string) error {
	// Check if installed via Homebrew
	if updater.IsHomebrew() {
		output.Info("Scuta was installed via Homebrew. Run:")
		output.Info("  brew upgrade scuta")
		return nil
	}

	scutaDir, err := path.ScutaDir()
	if err != nil {
		return err
	}

	token := auth.ResolveTokenWithConfig(scutaDir)
	ghClient := github.NewClient(token)
	upd := updater.New(ghClient)

	output.Info("Checking for updates...")

	update, err := upd.CheckSelfUpdate(version)
	if err != nil {
		return err
	}

	if update == nil {
		output.Success("scuta %s (already latest)", github.NormalizeVersion(version))
		return nil
	}

	output.Info("Update available: %s → %s", update.CurrentVersion, update.LatestVersion)

	// Download and install new binary
	inst := installer.New(ghClient, scutaDir)
	result, err := inst.Install("scuta", update.Repo, update.LatestVersion, true)
	if err != nil {
		return err
	}

	// Replace current binary
	currentExe, err := os.Executable()
	if err != nil {
		return err
	}

	// Write to .new, then rename over the original
	newPath := currentExe + ".new"
	data, err := os.ReadFile(result.BinaryPath)
	if err != nil {
		return err
	}

	if err := os.WriteFile(newPath, data, 0o755); err != nil {
		return err
	}

	if err := os.Rename(newPath, currentExe); err != nil {
		// Clean up the .new file if rename fails
		os.Remove(newPath)
		return err
	}

	output.Success("scuta %s → %s (updated)", update.CurrentVersion, update.LatestVersion)
	return nil
}
