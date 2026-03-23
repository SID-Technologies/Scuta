package cmd

import (
	"sort"

	"github.com/sid-technologies/scuta/lib/output"
	"github.com/sid-technologies/scuta/lib/path"
	"github.com/sid-technologies/scuta/lib/registry"
	"github.com/sid-technologies/scuta/lib/state"

	"github.com/spf13/cobra"
)

func ListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "Show all tools with install status",
		Long: `Displays all tools from the registry with their current install status,
installed version, and latest available version.`,
		RunE: runList,
	}

	return cmd
}

//nolint:gochecknoinits // Standard Cobra pattern
func init() {
	rootCmd.AddCommand(ListCmd())
}

func runList(_ *cobra.Command, _ []string) error {
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

	// Build merged install map (user + system state)
	// User installations take precedence over system ones.
	mergedTools := state.MergedTools(st, path.SystemStatePath())

	// Build a lookup: tool name → ToolEntry
	installMap := make(map[string]state.ToolEntry, len(mergedTools))
	for _, entry := range mergedTools {
		installMap[entry.Name] = entry
	}

	names := reg.Names()
	sort.Strings(names)

	// JSON output mode
	if output.IsJSON() {
		var tools []output.ToolInfo
		for _, name := range names {
			tool, _ := reg.Get(name)

			info := output.ToolInfo{
				Name:        name,
				Description: tool.Description,
				Repo:        tool.Repo,
				Source:      reg.Source(name),
			}

			if entry, installed := installMap[name]; installed {
				info.Installed = entry.Version
				info.Status = "installed"
				info.InstallSource = entry.Source
			} else {
				info.Installed = ""
				info.Status = "not installed"
			}

			tools = append(tools, info)
		}

		output.JSON(tools)
		return nil
	}

	// Table output mode
	headers := []string{"TOOL", "VERSION", "STATUS", "INSTALL", "SOURCE", "DESCRIPTION"}
	var rows []output.TableRow

	for _, name := range names {
		tool, _ := reg.Get(name)

		versionStr := "-"
		statusStr := "not installed"
		installSource := "-"

		if entry, installed := installMap[name]; installed {
			versionStr = entry.Version
			statusStr = "installed"
			installSource = entry.Source
		}

		rows = append(rows, output.TableRow{
			Columns: []string{name, versionStr, statusStr, installSource, reg.Source(name), tool.Description},
		})
	}

	output.PrintTable(headers, rows)

	return nil
}
