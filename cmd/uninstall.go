package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"
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

func UninstallCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "uninstall [tool]",
		Short: "Remove an installed tool",
		Long: `Removes the tool binary from ~/.scuta/bin/ and clears its state entry.
Does not affect Homebrew-installed versions of the same tool.

Use --all to uninstall every installed tool at once.`,
		Args: cobra.MaximumNArgs(1),
		RunE: runUninstall,
	}

	cmd.Flags().Bool("all", false, "Uninstall all installed tools")
	cmd.Flags().Bool("dry-run", false, "Show what would be uninstalled without uninstalling")
	cmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompt for --all")

	return cmd
}

//nolint:gochecknoinits // Standard Cobra pattern
func init() {
	rootCmd.AddCommand(UninstallCmd())
}

func runUninstall(cmd *cobra.Command, args []string) error {
	allFlag, _ := cmd.Flags().GetBool("all")
	dryRunFlag, _ := cmd.Flags().GetBool("dry-run")
	yesFlag, _ := cmd.Flags().GetBool("yes")

	scutaDir, err := path.ScutaDir()
	if err != nil {
		return err
	}

	st, err := state.Load(scutaDir)
	if err != nil {
		return err
	}

	// Determine which tools to uninstall
	var toolNames []string

	if allFlag {
		for name := range st.Tools {
			toolNames = append(toolNames, name)
		}
		if len(toolNames) == 0 {
			output.Info("No tools installed")
			return nil
		}
	} else if len(args) == 1 {
		toolNames = []string{args[0]}
	} else {
		output.Error("specify a tool name or use --all")
		return nil
	}

	// Validate tool names against registry for single-tool uninstall
	if !allFlag {
		reg, err := registry.Load()
		if err != nil {
			return err
		}

		toolName := toolNames[0]
		if _, ok := reg.Get(toolName); !ok {
			suggestion := suggest.FormatSuggestion(toolName, reg.Names())
			if suggestion != "" {
				output.Error("unknown tool %q — %s", toolName, suggestion)
			} else {
				output.Error("unknown tool %q. Run 'scuta list' to see available tools", toolName)
			}
			return nil
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
	}

	// Dry-run for --all
	if allFlag && dryRunFlag {
		for _, name := range toolNames {
			ts, _ := st.GetTool(name)
			output.Info("[dry run] Would uninstall %s %s", name, ts.Version)
		}
		return nil
	}

	// Confirmation prompt for --all (unless --yes)
	if allFlag && !yesFlag {
		fmt.Printf("Uninstall all %d tools? (y/N) ", len(toolNames))
		reader := bufio.NewReader(os.Stdin)
		input, err := reader.ReadString('\n')
		if err != nil {
			return err
		}
		input = strings.TrimSpace(strings.ToLower(input))
		if input != "y" && input != "yes" {
			output.Info("Aborted")
			return nil
		}
	}

	start := time.Now()
	ghClient := github.NewClient("")
	inst := installer.New(ghClient, scutaDir)

	var toolResults []history.ToolResult
	successCount := 0

	for _, toolName := range toolNames {
		ts, installed := st.GetTool(toolName)
		if !installed {
			continue
		}

		toolStart := time.Now()
		output.Info("Uninstalling %s %s...", toolName, ts.Version)

		if err := inst.Uninstall(toolName); err != nil {
			output.Error("Failed to uninstall %s: %v", toolName, err)
			toolResults = append(toolResults, history.ToolResult{
				Name:     toolName,
				Action:   "uninstall",
				Version:  ts.Version,
				Success:  false,
				Duration: time.Since(toolStart).Round(time.Millisecond).String(),
				Error:    err.Error(),
			})
			continue
		}

		st.RemoveTool(toolName)
		output.Success("Uninstalled %s", toolName)

		toolResults = append(toolResults, history.ToolResult{
			Name:     toolName,
			Action:   "uninstall",
			Version:  ts.Version,
			Success:  true,
			Duration: time.Since(toolStart).Round(time.Millisecond).String(),
		})
		successCount++
	}

	// Save state
	if err := st.Save(scutaDir); err != nil {
		output.Error("Failed to save state: %v", err)
	}

	// Record history
	allSuccess := successCount == len(toolResults)
	entry := history.NewEntry("uninstall", allSuccess, time.Since(start), toolResults)
	if err := history.Record(scutaDir, entry); err != nil {
		output.Debugf("Failed to record history: %v", err)
	}

	if allFlag && len(toolNames) > 1 {
		output.Info("%d/%d tools uninstalled successfully", successCount, len(toolNames))
	}

	return nil
}
