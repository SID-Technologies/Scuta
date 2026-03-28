package cmd

import (
	"fmt"
	"sort"
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
	"github.com/sid-technologies/scuta/lib/updater"

	"github.com/spf13/cobra"
)

func UpdateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update [tool]",
		Short: "Update tools to their latest versions",
		Long: `With no arguments, updates ALL installed tools AND scuta itself.
With a tool name, updates only that tool.`,
		Args: cobra.MaximumNArgs(1),
		RunE: runUpdate,
	}

	cmd.Flags().Bool("skip-verify", false, "Skip checksum verification")
	cmd.Flags().Bool("dry-run", false, "Show what would be updated without updating")
	cmd.Flags().Bool("system", false, "Update system-wide installations (requires root/admin)")

	return cmd
}

//nolint:gochecknoinits // Standard Cobra pattern
func init() {
	rootCmd.AddCommand(UpdateCmd())
}

func runUpdate(cmd *cobra.Command, args []string) error {
	ctx, cleanup := helper.WithSignalCancel(cmd.Context())
	defer cleanup()

	skipVerifyFlag, _ := cmd.Flags().GetBool("skip-verify")
	dryRunFlag, _ := cmd.Flags().GetBool("dry-run")
	systemFlag, _ := cmd.Flags().GetBool("system")

	if systemFlag && !isRoot() {
		return exitcodes.NewError(exitcodes.General, "system-wide update requires root/admin privileges (try sudo)")
	}

	reg, err := registry.Load()
	if err != nil {
		return err
	}

	scutaDir, err := path.ScutaDir()
	if err != nil {
		return err
	}

	var stateDir string
	if systemFlag {
		stateDir = path.SystemStateDir()
	} else {
		stateDir = scutaDir
	}

	st, err := state.Load(stateDir)
	if err != nil {
		return err
	}

	token := auth.ResolveTokenWithConfig(scutaDir)
	ghClient := newGitHubClient(token, scutaDir)
	upd := updater.New(ghClient)

	// Determine which tools to update
	var toolNames []string
	if len(args) == 1 {
		toolName := args[0]

		// Check if it's installed first
		ts, installed := st.GetTool(toolName)
		if !installed {
			// Not installed — check registry for suggestion
			if _, ok := reg.Get(toolName); !ok {
				suggestion := suggest.FormatSuggestion(toolName, reg.Names())
				if suggestion != "" {
					return exitcodes.NewError(exitcodes.InvalidArgs, fmt.Sprintf("unknown tool %q — %s", toolName, suggestion))
				}
			}
			return exitcodes.NewError(exitcodes.InvalidArgs, fmt.Sprintf("%s is not installed. Run 'scuta install %s' first", toolName, toolName))
		}

		// Validate tool exists in registry OR has a repo in state (direct-installed)
		if _, ok := reg.Get(toolName); !ok && ts.Repo == "" {
			return exitcodes.NewError(exitcodes.InvalidArgs, fmt.Sprintf("tool %q is installed but not in registry and has no repo info — reinstall with 'scuta install owner/repo'", toolName))
		}

		toolNames = []string{toolName}
	} else {
		// Update all installed tools (registry + direct-installed with repo info)
		for name, ts := range st.Tools {
			if _, ok := reg.Get(name); ok {
				toolNames = append(toolNames, name)
			} else if ts.Repo != "" {
				toolNames = append(toolNames, name)
			}
		}
		sort.Strings(toolNames)
		if len(toolNames) == 0 {
			output.Info("No tools installed. Run 'scuta install --all' first")
			return nil
		}
	}

	output.Info("Checking for updates...")

	// Check which tools have updates
	updates := upd.CheckForUpdates(ctx, st.Tools, reg.Tools)

	// Filter to only requested tools
	updateMap := make(map[string]updater.UpdateAvailable)
	for _, u := range updates {
		updateMap[u.Name] = u
	}

	var toUpdate []updater.UpdateAvailable
	for _, name := range toolNames {
		if u, ok := updateMap[name]; ok {
			toUpdate = append(toUpdate, u)
		} else {
			ts, _ := st.GetTool(name)
			output.Success("%s %s is already the latest", name, ts.Version)
		}
	}

	if len(toUpdate) == 0 {
		output.Info("All tools are up to date")

		// Telemetry (best-effort)
		cfg, cfgErr := config.LoadWithMerge(scutaDir)
		if cfgErr == nil {
			_ = telemetry.Record(scutaDir, cfg.Telemetry, "update")
		}
		return nil
	}

	// Dry-run: print what would be updated and exit
	if dryRunFlag {
		for _, u := range toUpdate {
			output.Info("[dry run] Would update %s %s → %s", u.Name, u.CurrentVersion, u.LatestVersion)
		}
		return nil
	}

	// Acquire lock for updates
	updateToolNames := make([]string, len(toUpdate))
	for i, u := range toUpdate {
		updateToolNames[i] = u.Name
	}

	if err := lock.Acquire(scutaDir, "update", updateToolNames, false); err != nil {
		return err
	}
	defer lock.Release(scutaDir)

	var inst *installer.Installer
	if systemFlag {
		inst = installer.NewWithBinDir(ghClient, scutaDir, path.SystemBinDir())
	} else {
		inst = installer.New(ghClient, scutaDir)
	}
	pol := loadPolicy(scutaDir)
	start := time.Now()
	var toolResults []history.ToolResult
	successCount := 0

	for _, u := range toUpdate {
		tool, inRegistry := reg.Get(u.Name)
		toolStart := time.Now()

		// Determine repo: prefer registry, fall back to update info (which includes state repo)
		repo := u.Repo
		if inRegistry {
			repo = tool.Repo
		}

		// Policy check before updating
		if v := pol.CheckToolVersion(u.Name, u.LatestVersion); v != nil {
			output.Error("Policy violation for %s: %s", u.Name, v.Message)
			toolResults = append(toolResults, history.ToolResult{
				Name:     u.Name,
				Action:   "update",
				Success:  false,
				Duration: time.Since(toolStart).Round(time.Millisecond).String(),
				Error:    v.Message,
			})
			continue
		}

		output.Info("Updating %s %s → %s...", u.Name, u.CurrentVersion, u.LatestVersion)

		if ctx.Err() != nil {
			break
		}

		var result *installer.InstallResult
		var err error

		if inRegistry && hasExtendedOpts(tool) {
			opts := buildInstallOpts(tool)
			result, err = inst.InstallWithOpts(ctx, u.Name, repo, "", true, skipVerifyFlag, opts)
		} else {
			result, err = inst.Install(ctx, u.Name, repo, "", true, skipVerifyFlag)
		}
		if err != nil {
			output.Error("Failed to update %s: %v", u.Name, err)
			toolResults = append(toolResults, history.ToolResult{
				Name:     u.Name,
				Action:   "update",
				Success:  false,
				Duration: time.Since(toolStart).Round(time.Millisecond).String(),
				Error:    err.Error(),
			})
			continue
		}

		st.SetTool(u.Name, state.ToolState{
			Version:     result.Version,
			InstalledAt: st.Tools[u.Name].InstalledAt,
			UpdatedAt:   time.Now(),
			BinaryPath:  result.BinaryPath,
			Repo:        st.Tools[u.Name].Repo,
		})

		output.Success("Updated %s %s → %s", u.Name, u.CurrentVersion, result.Version)
		toolResults = append(toolResults, history.ToolResult{
			Name:     u.Name,
			Action:   "update",
			Version:  result.Version,
			Success:  true,
			Duration: time.Since(toolStart).Round(time.Millisecond).String(),
		})
		successCount++
	}

	// Save state (to system dir for --system, user dir otherwise)
	st.LastUpdateCheck = time.Now()
	if err := st.Save(stateDir); err != nil {
		output.Error("Failed to save state: %v", err)
	}

	// Record history
	allSuccess := successCount == len(toUpdate)
	entry := history.NewEntry("update", allSuccess, time.Since(start), toolResults)
	if err := history.Record(scutaDir, entry); err != nil {
		output.Debugf("Failed to record history: %v", err)
	}

	// Telemetry (best-effort, uses merged config for effective settings)
	cfg, cfgErr := config.LoadWithMerge(scutaDir)
	if cfgErr == nil {
		_ = telemetry.Record(scutaDir, cfg.Telemetry, "update")
	}

	if len(toUpdate) > 1 {
		output.Info("%d/%d tools updated successfully", successCount, len(toUpdate))
	}

	return nil
}
