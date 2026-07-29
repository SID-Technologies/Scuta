package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/sid-technologies/scuta/lib/auth"
	"github.com/sid-technologies/scuta/lib/config"
	"github.com/sid-technologies/scuta/lib/exitcodes"
	"github.com/sid-technologies/scuta/lib/helper"
	"github.com/sid-technologies/scuta/lib/history"
	"github.com/sid-technologies/scuta/lib/installer"
	"github.com/sid-technologies/scuta/lib/lock"
	"github.com/sid-technologies/scuta/lib/manifest"
	"github.com/sid-technologies/scuta/lib/output"
	"github.com/sid-technologies/scuta/lib/path"
	"github.com/sid-technologies/scuta/lib/policy"
	"github.com/sid-technologies/scuta/lib/registry"
	"github.com/sid-technologies/scuta/lib/state"
	"github.com/sid-technologies/scuta/lib/telemetry"

	"github.com/spf13/cobra"
)

func SyncCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Reconcile installed tools to a manifest",
		Long: `Reconciles this machine to a declarative manifest (scuta.lock.yaml).

The manifest lists the tools — and optionally pinned versions, repos, and
binary names — that should be installed. Sync installs anything missing,
changes tools that are at the wrong pinned version, and (with --prune) removes
tools not in the manifest.

Example manifest:

  tools:
    # Registry tool, pinned version (shorthand form):
    pilum: "0.7.5"
    # Arbitrary public repo whose binary name differs from the repo name:
    ripgrep:
      version: "14.1.0"
      repo: "BurntSushi/ripgrep"
      bin: "rg"
    # Unpinned — installed only when missing, kept current by 'scuta update':
    bat: "latest"

This makes the org's toolset reproducible: commit the manifest, and every
engineer runs 'scuta sync' to converge to the same set and versions.`,
		Args: cobra.NoArgs,
		RunE: runSync,
	}

	cmd.Flags().StringP("file", "f", "", "Path to the manifest (default: scuta.lock.yaml in the current directory)")
	cmd.Flags().Bool("prune", false, "Remove installed tools that are not in the manifest")
	cmd.Flags().Bool("dry-run", false, "Show the reconciliation plan without applying it")
	cmd.Flags().Bool("skip-verify", false, "Skip checksum verification")

	return cmd
}

//nolint:gochecknoinits // Standard Cobra pattern
func init() {
	rootCmd.AddCommand(SyncCmd())
}

func runSync(cmd *cobra.Command, _ []string) error {
	ctx, cleanup := helper.WithSignalCancel(cmd.Context())
	defer cleanup()

	fileFlag, _ := cmd.Flags().GetString("file")
	pruneFlag, _ := cmd.Flags().GetBool("prune")
	dryRunFlag, _ := cmd.Flags().GetBool("dry-run")
	skipVerifyFlag, _ := cmd.Flags().GetBool("skip-verify")

	manifestPath := fileFlag
	if manifestPath == "" {
		manifestPath = manifest.FindDefault("")
	}
	if manifestPath == "" {
		return exitcodes.NewError(exitcodes.InvalidArgs,
			"no manifest found — create scuta.lock.yaml or pass --file <path>")
	}

	man, err := manifest.Load(manifestPath)
	if err != nil {
		return exitcodes.NewError(exitcodes.InvalidArgs, err.Error())
	}

	scutaDir, err := path.ScutaDir()
	if err != nil {
		return err
	}

	reg, err := registry.Load()
	if err != nil {
		return err
	}

	st, err := state.Load(scutaDir)
	if err != nil {
		return err
	}

	// Build current installed version map for the planner.
	installed := make(map[string]string, len(st.Tools))
	for name, ts := range st.Tools {
		installed[name] = ts.Version
	}

	actions := man.Plan(installed, pruneFlag)
	changes := manifest.Changes(actions)

	output.Info("Manifest: %s", manifestPath)

	if len(changes) == 0 {
		output.Success("Already in sync — %d tool(s) match the manifest", len(actions))
		return nil
	}

	printSyncPlan(changes)

	if dryRunFlag {
		return nil
	}

	// Acquire lock across all affected tools.
	names := make([]string, len(changes))
	for i, a := range changes {
		names[i] = a.Name
	}
	if err := lock.Acquire(scutaDir, "sync", names, false); err != nil {
		return err
	}
	defer lock.Release(scutaDir)

	token := auth.ResolveTokenWithConfig(scutaDir)
	ghClient := newGitHubClient(token, scutaDir)
	inst := installer.New(ghClient, scutaDir)
	pol := loadPolicy(scutaDir)

	start := time.Now()
	var results []history.ToolResult
	successCount := 0

	for _, a := range changes {
		if ctx.Err() != nil {
			break
		}
		result := applySyncAction(ctx, a, reg, st, inst, pol, skipVerifyFlag)
		results = append(results, result)
		if result.Success {
			successCount++
		}
	}

	if err := st.Save(scutaDir); err != nil {
		output.Error("Failed to save state: %v", err)
	}

	allSuccess := successCount == len(changes)
	entry := history.NewEntry("sync", allSuccess, time.Since(start), results)
	if err := history.Record(scutaDir, entry); err != nil {
		output.Debugf("Failed to record history: %v", err)
	}

	if cfg, cfgErr := config.LoadWithMerge(scutaDir); cfgErr == nil {
		_ = telemetry.Record(scutaDir, cfg.Telemetry, "sync")
	}

	output.Info("%d/%d change(s) applied successfully", successCount, len(changes))

	if !allSuccess {
		return exitcodes.NewError(exitcodes.Install, "sync completed with errors")
	}
	return nil
}

func printSyncPlan(changes []manifest.Action) {
	for _, a := range changes {
		switch a.Type {
		case manifest.ActionInstall:
			output.Info("  + install %s %s", a.Name, targetLabel(a))
		case manifest.ActionChange:
			output.Info("  ~ change  %s %s → %s", a.Name, a.CurrentVersion, targetLabel(a))
		case manifest.ActionRemove:
			output.Info("  - remove  %s %s", a.Name, a.CurrentVersion)
		default:
			// ActionUpToDate is filtered out before printing; nothing to show.
		}
	}
}

func targetLabel(a manifest.Action) string {
	if a.IsLatest {
		return "(latest)"
	}
	return a.TargetVersion
}

// resolveSyncRepo determines which repo to install a tool from: manifest entry
// override first, then the registry, then existing state.
func resolveSyncRepo(a manifest.Action, reg *registry.Registry, st *state.State) string {
	if a.Repo != "" {
		return a.Repo
	}
	if tool, ok := reg.Get(a.Name); ok && tool.Repo != "" {
		return tool.Repo
	}
	if ts, ok := st.GetTool(a.Name); ok {
		return ts.Repo
	}
	return ""
}

// applySyncAction executes one reconciliation step.
func applySyncAction(
	ctx context.Context,
	a manifest.Action,
	reg *registry.Registry,
	st *state.State,
	inst *installer.Installer,
	pol *policy.Policy,
	skipVerify bool,
) history.ToolResult {
	toolStart := time.Now()
	res := history.ToolResult{Name: a.Name, Action: a.Type.String()}
	finish := func() history.ToolResult {
		res.Duration = time.Since(toolStart).Round(time.Millisecond).String()
		return res
	}

	if a.Type == manifest.ActionRemove {
		output.Info("Removing %s %s...", a.Name, a.CurrentVersion)
		if err := uninstallTool(inst, st, a.Name); err != nil {
			output.Error("Failed to remove %s: %v", a.Name, err)
			res.Error = err.Error()
			return finish()
		}
		st.RemoveTool(a.Name)
		res.Version = a.CurrentVersion
		res.Success = true
		output.Success("Removed %s", a.Name)
		return finish()
	}

	// Policy check for pinned versions.
	if !a.IsLatest {
		if v := pol.CheckToolVersion(a.Name, a.TargetVersion); v != nil {
			output.Error("Policy violation for %s: %s", a.Name, v.Message)
			res.Error = v.Message
			return finish()
		}
	}

	repo := resolveSyncRepo(a, reg, st)
	if repo == "" {
		msg := fmt.Sprintf("no repo known for %q — add it to the registry or set 'repo' in the manifest", a.Name)
		output.Error(msg)
		res.Error = msg
		return finish()
	}

	output.Info("Syncing %s %s...", a.Name, targetLabel(a))

	tool, inRegistry := reg.Get(a.Name)
	directInstalled := !inRegistry || reg.Source(a.Name) == registry.SourceLocal

	var result *installer.InstallResult
	var err error
	switch {
	case inRegistry && hasExtendedOpts(tool):
		opts := buildInstallOpts(tool)
		if a.Bin != "" {
			opts.BinName = a.Bin // manifest overrides the registry binary name
		}
		result, err = inst.InstallWithOpts(ctx, a.Name, repo, a.TargetVersion, true, skipVerify, opts)
	case directInstalled:
		result, err = inst.InstallWithOpts(ctx, a.Name, repo, a.TargetVersion, true, skipVerify,
			installer.InstallOpts{BestEffort: true, BinName: a.Bin})
	default:
		result, err = inst.Install(ctx, a.Name, repo, a.TargetVersion, true, skipVerify)
	}
	if err != nil {
		output.Error("Failed to sync %s: %v", a.Name, err)
		res.Error = err.Error()
		return finish()
	}

	// Enforce policy on the resolved version when the manifest said "latest".
	if a.IsLatest {
		if v := pol.CheckToolVersion(a.Name, result.Version); v != nil {
			output.Error("Policy violation for %s: %s", a.Name, v.Message)
			_ = inst.Uninstall(a.Name)
			res.Error = v.Message
			return finish()
		}
	}

	st.SetTool(a.Name, state.ToolState{
		Version:     result.Version,
		InstalledAt: time.Now(),
		BinaryPath:  result.BinaryPath,
		Repo:        repo,
	})
	res.Version = result.Version
	res.Success = true
	output.Success("Synced %s %s", a.Name, result.Version)
	return finish()
}
