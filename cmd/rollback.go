package cmd

import (
	"fmt"
	"time"

	"github.com/sid-technologies/scuta/lib/auth"
	"github.com/sid-technologies/scuta/lib/config"
	"github.com/sid-technologies/scuta/lib/exitcodes"
	"github.com/sid-technologies/scuta/lib/helper"
	"github.com/sid-technologies/scuta/lib/history"
	"github.com/sid-technologies/scuta/lib/installer"
	"github.com/sid-technologies/scuta/lib/lock"
	"github.com/sid-technologies/scuta/lib/output"
	"github.com/sid-technologies/scuta/lib/path"
	"github.com/sid-technologies/scuta/lib/registry"
	"github.com/sid-technologies/scuta/lib/state"
	"github.com/sid-technologies/scuta/lib/suggest"
	"github.com/sid-technologies/scuta/lib/telemetry"

	"github.com/spf13/cobra"
)

func RollbackCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rollback <tool>",
		Short: "Reinstall the previous version of a tool",
		Long: `Roll a tool back to the version it was at before the last install or
update, as recorded in history (~/.scuta/history.jsonl).

Running rollback twice returns to where you started (it walks history
chronologically). To pin an arbitrary older version instead, use:

  scuta install <tool> --version <version>`,
		Args: cobra.ExactArgs(1),
		RunE: runRollback,
	}

	cmd.Flags().Bool("skip-verify", false, "Skip checksum verification")
	cmd.Flags().Bool("dry-run", false, "Show what would happen without rolling back")

	return cmd
}

//nolint:gochecknoinits // Standard Cobra pattern
func init() {
	rootCmd.AddCommand(RollbackCmd())
}

//nolint:gocyclo // linear command flow mirroring runUpdate
func runRollback(cmd *cobra.Command, args []string) error {
	ctx, cleanup := helper.WithSignalCancel(cmd.Context())
	defer cleanup()

	skipVerifyFlag, _ := cmd.Flags().GetBool("skip-verify")
	dryRunFlag, _ := cmd.Flags().GetBool("dry-run")
	toolName := args[0]

	reg, err := registry.Load()
	if err != nil {
		return err
	}

	scutaDir, err := path.ScutaDir()
	if err != nil {
		return err
	}

	st, err := state.Load(scutaDir)
	if err != nil {
		return err
	}

	ts, installed := st.GetTool(toolName)
	if !installed {
		if _, ok := reg.Get(toolName); !ok {
			if suggestion := suggest.FormatSuggestion(toolName, reg.Names()); suggestion != "" {
				return exitcodes.NewError(exitcodes.InvalidArgs, fmt.Sprintf("unknown tool %q — %s", toolName, suggestion))
			}
		}
		return exitcodes.NewError(exitcodes.InvalidArgs, fmt.Sprintf("%s is not installed — nothing to roll back", toolName))
	}

	// Resolve the repo the same way update does: registry first, then the
	// repo recorded in state for direct-installed tools.
	tool, inRegistry := reg.Get(toolName)
	repo := ts.Repo
	if inRegistry {
		repo = tool.Repo
	}
	if repo == "" {
		return exitcodes.NewError(exitcodes.InvalidArgs, fmt.Sprintf("tool %q has no repo info — reinstall with 'scuta install owner/repo'", toolName))
	}

	entries, err := history.Load(scutaDir)
	if err != nil {
		return err
	}

	prevVersion, ok := history.PreviousVersion(entries, toolName, ts.Version)
	if !ok {
		return exitcodes.NewError(exitcodes.General, fmt.Sprintf(
			"no previous version of %s found in history (currently %s). Pin one explicitly: scuta install %s --version <version>",
			toolName, ts.Version, toolName))
	}

	if dryRunFlag {
		output.Info("[dry run] Would roll back %s %s → %s", toolName, ts.Version, prevVersion)
		return nil
	}

	// Policy check on the rollback target — a rollback must not reintroduce
	// a blocked or out-of-range version.
	pol := loadPolicy(scutaDir)
	if v := pol.CheckToolVersion(toolName, prevVersion); v != nil {
		return exitcodes.NewError(exitcodes.General, fmt.Sprintf("policy violation for %s: %s", toolName, v.Message))
	}

	if err := lock.Acquire(scutaDir, "rollback", []string{toolName}, false); err != nil {
		return err
	}
	defer lock.Release(scutaDir)

	token := auth.ResolveTokenWithConfig(scutaDir)
	ghClient := newGitHubClient(token, scutaDir)
	inst := installer.New(ghClient, scutaDir)
	applyDownloadCacheConfig(inst, scutaDir)

	output.Info("Rolling back %s %s → %s...", toolName, ts.Version, prevVersion)

	start := time.Now()
	var result *installer.InstallResult

	// Mirror install/update behavior: registry-blessed tools stay fail-closed
	// on checksums; direct-installed (local/unknown) tools are best-effort.
	directInstalled := !inRegistry || reg.Source(toolName) == registry.SourceLocal
	switch {
	case inRegistry && hasExtendedOpts(tool):
		result, err = inst.InstallWithOpts(ctx, toolName, repo, prevVersion, true, skipVerifyFlag, buildInstallOpts(tool))
	case directInstalled:
		result, err = inst.InstallWithOpts(ctx, toolName, repo, prevVersion, true, skipVerifyFlag, installer.InstallOpts{BestEffort: true})
	default:
		result, err = inst.Install(ctx, toolName, repo, prevVersion, true, skipVerifyFlag)
	}

	toolResult := history.ToolResult{
		Name:     toolName,
		Action:   "rollback",
		Duration: time.Since(start).Round(time.Millisecond).String(),
	}

	if err != nil {
		toolResult.Error = err.Error()
		entry := history.NewEntry("rollback", false, time.Since(start), []history.ToolResult{toolResult})
		if recErr := history.Record(scutaDir, entry); recErr != nil {
			output.Debugf("Failed to record history: %v", recErr)
		}
		return err
	}

	st.SetTool(toolName, state.ToolState{
		Version:     result.Version,
		InstalledAt: ts.InstalledAt,
		UpdatedAt:   time.Now(),
		BinaryPath:  result.BinaryPath,
		Repo:        ts.Repo,
		Sha256:      result.Sha256,
		Verified:    result.Verified,
	})
	if err := st.Save(scutaDir); err != nil {
		output.Error("Failed to save state: %v", err)
	}

	toolResult.Version = result.Version
	toolResult.Success = true
	entry := history.NewEntry("rollback", true, time.Since(start), []history.ToolResult{toolResult})
	if err := history.Record(scutaDir, entry); err != nil {
		output.Debugf("Failed to record history: %v", err)
	}

	cfg, cfgErr := config.LoadWithMerge(scutaDir)
	if cfgErr == nil {
		_ = telemetry.Record(scutaDir, cfg.Telemetry, "rollback")
	}

	output.Success("Rolled back %s %s → %s", toolName, ts.Version, result.Version)

	return nil
}
