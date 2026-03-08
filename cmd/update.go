package cmd

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sid-technologies/scuta/lib/auth"
	"github.com/sid-technologies/scuta/lib/github"
	"github.com/sid-technologies/scuta/lib/history"
	"github.com/sid-technologies/scuta/lib/installer"
	"github.com/sid-technologies/scuta/lib/lock"
	"github.com/sid-technologies/scuta/lib/output"
	"github.com/sid-technologies/scuta/lib/path"
	"github.com/sid-technologies/scuta/lib/registry"
	"github.com/sid-technologies/scuta/lib/state"
	"github.com/sid-technologies/scuta/lib/suggest"
	"github.com/sid-technologies/scuta/lib/updater"

	"github.com/spf13/cobra"
)

var updateCmd = &cobra.Command{
	Use:   "update [tool]",
	Short: "Update tools to their latest versions",
	Long: `With no arguments, updates ALL installed tools AND scuta itself.
With a tool name, updates only that tool.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runUpdate,
}

//nolint:gochecknoinits // Standard Cobra pattern
func init() {
	updateCmd.Flags().Bool("skip-verify", false, "Skip checksum verification")
	updateCmd.Flags().Bool("dry-run", false, "Show what would be updated without updating")
	rootCmd.AddCommand(updateCmd)
}

func runUpdate(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithCancel(cmd.Context())
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		output.Warning("\nInterrupted, cleaning up...")
		cancel()
	}()
	defer signal.Stop(sigChan)
	defer cancel()

	skipVerifyFlag, _ := cmd.Flags().GetBool("skip-verify")
	dryRunFlag, _ := cmd.Flags().GetBool("dry-run")

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

	token := auth.ResolveTokenWithConfig(scutaDir)
	ghClient := github.NewClient(token)
	upd := updater.New(ghClient)

	// Determine which tools to update
	var toolNames []string
	if len(args) == 1 {
		toolName := args[0]

		// Validate tool exists in registry
		if _, ok := reg.Get(toolName); !ok {
			suggestion := suggest.FormatSuggestion(toolName, reg.Names())
			if suggestion != "" {
				output.Error("unknown tool %q — %s", toolName, suggestion)
			} else {
				output.Error("unknown tool %q. Run 'scuta list' to see available tools", toolName)
			}
			return nil
		}

		// Check if it's installed
		if _, installed := st.GetTool(toolName); !installed {
			output.Error("%s is not installed. Run 'scuta install %s' first", toolName, toolName)
			return nil
		}

		toolNames = []string{toolName}
	} else {
		// Update all installed tools
		for name := range st.Tools {
			if _, ok := reg.Get(name); ok {
				toolNames = append(toolNames, name)
			}
		}
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

	inst := installer.New(ghClient, scutaDir)
	start := time.Now()
	var toolResults []history.ToolResult
	successCount := 0

	for _, u := range toUpdate {
		tool, _ := reg.Get(u.Name)
		toolStart := time.Now()

		output.Info("Updating %s %s → %s...", u.Name, u.CurrentVersion, u.LatestVersion)

		if ctx.Err() != nil {
			break
		}

		result, err := inst.Install(ctx, u.Name, tool.Repo, "", true, skipVerifyFlag)
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

	// Save state
	st.LastUpdateCheck = time.Now()
	if err := st.Save(scutaDir); err != nil {
		output.Error("Failed to save state: %v", err)
	}

	// Record history
	allSuccess := successCount == len(toUpdate)
	entry := history.NewEntry("update", allSuccess, time.Since(start), toolResults)
	if err := history.Record(scutaDir, entry); err != nil {
		output.Debugf("Failed to record history: %v", err)
	}

	if len(toUpdate) > 1 {
		output.Info("%d/%d tools updated successfully", successCount, len(toUpdate))
	}

	return nil
}
