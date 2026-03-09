package cmd

import (
	"fmt"
	"strings"

	"github.com/sid-technologies/scuta/lib/history"
	"github.com/sid-technologies/scuta/lib/output"
	"github.com/sid-technologies/scuta/lib/path"

	"github.com/spf13/cobra"
)

func HistoryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "history",
		Short: "Show recent install/update/uninstall operations",
		Long: `Displays the audit trail of tool operations from ~/.scuta/history.jsonl.

Use --limit to control how many entries are shown, or --tool to filter
by a specific tool name.`,
		RunE: runHistory,
	}

	cmd.Flags().IntP("limit", "n", 10, "Maximum number of entries to show")
	cmd.Flags().StringP("tool", "t", "", "Filter by tool name")

	return cmd
}

//nolint:gochecknoinits // Standard Cobra pattern
func init() {
	rootCmd.AddCommand(HistoryCmd())
}

func runHistory(cmd *cobra.Command, _ []string) error {
	limitFlag, _ := cmd.Flags().GetInt("limit")
	toolFlag, _ := cmd.Flags().GetString("tool")

	scutaDir, err := path.ScutaDir()
	if err != nil {
		return err
	}

	entries, err := history.Load(scutaDir)
	if err != nil {
		return err
	}

	if len(entries) == 0 {
		output.Info("No history recorded yet")
		return nil
	}

	// Filter by tool name if specified
	if toolFlag != "" {
		entries = filterByTool(entries, toolFlag)
		if len(entries) == 0 {
			output.Info("No history found for tool %q", toolFlag)
			return nil
		}
	}

	// Apply limit
	if limitFlag > 0 && len(entries) > limitFlag {
		entries = entries[:limitFlag]
	}

	// JSON output
	if output.IsJSON() {
		output.JSON(entries)
		return nil
	}

	// Table output
	headers := []string{"ID", "TIMESTAMP", "COMMAND", "STATUS", "TOOLS", "DURATION"}
	rows := make([]output.TableRow, len(entries))

	for i, entry := range entries {
		// Format timestamp in local time
		ts := entry.Timestamp.Local().Format("2006-01-02 15:04")

		// Status with colored symbol
		status := fmt.Sprintf("%s%s%s", output.SuccessColor, output.SymbolSuccess, output.Reset)
		if !entry.Success {
			status = fmt.Sprintf("%s%s%s", output.ErrorColor, output.SymbolFailure, output.Reset)
		}

		// Tool count
		toolCount := fmt.Sprintf("%d", len(entry.Tools))

		// Tool names for context
		if len(entry.Tools) > 0 && len(entry.Tools) <= 3 {
			names := make([]string, len(entry.Tools))
			for j, t := range entry.Tools {
				names[j] = t.Name
			}
			toolCount = strings.Join(names, ", ")
		}

		rows[i] = output.TableRow{
			Columns: []string{
				fmt.Sprintf("%s%s%s", output.Muted, entry.ID, output.Reset),
				ts,
				entry.Command,
				status,
				toolCount,
				entry.Duration,
			},
		}
	}

	output.PrintTable(headers, rows)
	return nil
}

// filterByTool returns only entries that contain a result for the given tool name.
func filterByTool(entries []history.Entry, toolName string) []history.Entry {
	var filtered []history.Entry
	for _, entry := range entries {
		for _, t := range entry.Tools {
			if t.Name == toolName {
				filtered = append(filtered, entry)
				break
			}
		}
	}
	return filtered
}
