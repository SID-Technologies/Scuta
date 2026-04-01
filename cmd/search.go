package cmd

import (
	"fmt"

	"github.com/sid-technologies/scuta/lib/exitcodes"
	"github.com/sid-technologies/scuta/lib/output"
	"github.com/sid-technologies/scuta/lib/registry"
	"github.com/sid-technologies/scuta/lib/suggest"

	"github.com/spf13/cobra"
)

func SearchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search for tools in the registry",
		Long: `Fuzzy search across tool names and descriptions.

Examples:
  scuta search fuzzy         # finds fzf
  scuta search "code gen"    # finds tools with "code gen" in description
  scuta search lint          # finds golangci-lint, etc.`,
		Args: cobra.ExactArgs(1),
		RunE: runSearch,
	}

	cmd.Flags().IntP("limit", "n", 10, "Maximum number of results")

	return cmd
}

//nolint:gochecknoinits // Standard Cobra pattern
func init() {
	rootCmd.AddCommand(SearchCmd())
}

func runSearch(_ *cobra.Command, args []string) error {
	query := args[0]

	reg, err := registry.Load()
	if err != nil {
		return err
	}

	// Build search entries from registry
	entries := make(map[string]suggest.ToolEntry, len(reg.Tools))
	for name, tool := range reg.Tools {
		entries[name] = suggest.ToolEntry{
			Description: tool.Description,
		}
	}

	results := suggest.Search(query, entries, 20)
	if len(results) == 0 {
		return exitcodes.NewError(exitcodes.InvalidArgs, fmt.Sprintf("no tools matching %q", query))
	}

	for _, r := range results {
		tool, _ := reg.Get(r.Name)
		output.Info("%-20s %s", r.Name, tool.Description)
	}

	return nil
}
