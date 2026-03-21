package cmd

import (
	"github.com/sid-technologies/scuta/lib/auth"
	"github.com/sid-technologies/scuta/lib/config"
	"github.com/sid-technologies/scuta/lib/github"
	"github.com/sid-technologies/scuta/lib/installer"
	"github.com/sid-technologies/scuta/lib/lock"
	"github.com/sid-technologies/scuta/lib/output"
	"github.com/sid-technologies/scuta/lib/path"
	"github.com/sid-technologies/scuta/lib/telemetry"
	"github.com/sid-technologies/scuta/lib/updater"

	"github.com/spf13/cobra"
)

func SelfUpdateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "self-update",
		Short: "Update Scuta itself",
		Long: `Downloads the latest Scuta release and replaces the current binary.
If installed via Homebrew, prints guidance to use brew upgrade instead.`,
		RunE: runSelfUpdate,
	}

	return cmd
}

//nolint:gochecknoinits // Standard Cobra pattern
func init() {
	rootCmd.AddCommand(SelfUpdateCmd())
}

func runSelfUpdate(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
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
	ghClient := newGitHubClient(token, scutaDir)
	upd := updater.New(ghClient)

	output.Info("Checking for updates...")

	update, err := upd.CheckSelfUpdate(ctx, version)
	if err != nil {
		return err
	}

	if update == nil {
		output.Success("scuta %s (already latest)", github.NormalizeVersion(version))
		return nil
	}

	output.Info("Update available: %s → %s", update.CurrentVersion, update.LatestVersion)

	// Acquire lock before installing
	if err := lock.Acquire(scutaDir, "self-update", []string{"scuta"}, false); err != nil {
		return err
	}
	defer lock.Release(scutaDir)

	// Download and install new binary
	inst := installer.New(ghClient, scutaDir)
	result, err := inst.Install(ctx, "scuta", update.Repo, update.LatestVersion, true, false)
	if err != nil {
		return err
	}

	// Replace current binary
	if err := updater.ReplaceBinary(result.BinaryPath); err != nil {
		return err
	}

	output.Success("scuta %s → %s (updated)", update.CurrentVersion, update.LatestVersion)

	// Telemetry (best-effort)
	cfg, cfgErr := config.LoadWithMerge(scutaDir)
	if cfgErr == nil {
		_ = telemetry.Record(scutaDir, cfg.Telemetry, "self-update")
	}

	return nil
}
