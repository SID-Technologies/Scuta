package cmd

import (
	"time"

	"github.com/sid-technologies/scuta/lib/github"
	"github.com/sid-technologies/scuta/lib/history"
	"github.com/sid-technologies/scuta/lib/installer"
	"github.com/sid-technologies/scuta/lib/output"
	"github.com/sid-technologies/scuta/lib/path"
	"github.com/sid-technologies/scuta/lib/registry"
	"github.com/sid-technologies/scuta/lib/state"
	"github.com/sid-technologies/scuta/lib/suggest"

	"github.com/spf13/cobra"
)

var uninstallCmd = &cobra.Command{
	Use:   "uninstall <tool>",
	Short: "Remove an installed tool",
	Long: `Removes the tool binary from ~/.scuta/bin/ and clears its state entry.
Does not affect Homebrew-installed versions of the same tool.`,
	Args: cobra.ExactArgs(1),
	RunE: runUninstall,
}

//nolint:gochecknoinits // Standard Cobra pattern
func init() {
	uninstallCmd.Flags().Bool("dry-run", false, "Show what would be uninstalled without uninstalling")
	rootCmd.AddCommand(uninstallCmd)
}

func runUninstall(cmd *cobra.Command, args []string) error {
	dryRunFlag, _ := cmd.Flags().GetBool("dry-run")
	toolName := args[0]

	reg, err := registry.Load()
	if err != nil {
		return err
	}

	// Validate tool name
	if _, ok := reg.Get(toolName); !ok {
		suggestion := suggest.FormatSuggestion(toolName, reg.Names())
		if suggestion != "" {
			output.Error("unknown tool %q — %s", toolName, suggestion)
		} else {
			output.Error("unknown tool %q. Run 'scuta list' to see available tools", toolName)
		}
		return nil
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
		output.Warning("%s is not installed", toolName)
		return nil
	}

	if dryRunFlag {
		output.Info("[dry run] Would uninstall %s %s", toolName, ts.Version)
		return nil
	}

	start := time.Now()

	ghClient := github.NewClient("")
	inst := installer.New(ghClient, scutaDir)

	output.Info("Uninstalling %s %s...", toolName, ts.Version)

	if err := inst.Uninstall(toolName); err != nil {
		output.Error("Failed to uninstall %s: %v", toolName, err)
		return nil
	}

	st.RemoveTool(toolName)
	if err := st.Save(scutaDir); err != nil {
		output.Error("Failed to save state: %v", err)
	}

	// Record history
	entry := history.NewEntry("uninstall", true, time.Since(start), []history.ToolResult{
		{
			Name:     toolName,
			Action:   "uninstall",
			Version:  ts.Version,
			Success:  true,
			Duration: time.Since(start).Round(time.Millisecond).String(),
		},
	})
	if err := history.Record(scutaDir, entry); err != nil {
		output.Debugf("Failed to record history: %v", err)
	}

	output.Success("Uninstalled %s", toolName)
	return nil
}
