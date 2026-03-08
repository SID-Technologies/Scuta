package cmd

import (
	"sort"

	"github.com/sid-technologies/scuta/lib/output"
	"github.com/sid-technologies/scuta/lib/path"
	"github.com/sid-technologies/scuta/lib/registry"
	"github.com/sid-technologies/scuta/lib/state"

	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "Show all tools with install status",
	Long: `Displays all tools from the registry with their current install status,
installed version, and latest available version.`,
	RunE: runList,
}

//nolint:gochecknoinits // Standard Cobra pattern
func init() {
	rootCmd.AddCommand(listCmd)
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

	names := reg.Names()
	sort.Strings(names)

	// JSON output mode
	if output.IsJSON() {
		type toolInfo struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			Repo        string `json:"repo"`
			Installed   string `json:"installed"`
			Status      string `json:"status"`
		}

		var tools []toolInfo
		for _, name := range names {
			tool, _ := reg.Get(name)
			ts, installed := st.GetTool(name)

			info := toolInfo{
				Name:        name,
				Description: tool.Description,
				Repo:        tool.Repo,
			}

			if installed {
				info.Installed = ts.Version
				info.Status = "installed"
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
	headers := []string{"TOOL", "VERSION", "STATUS", "DESCRIPTION"}
	var rows []output.TableRow

	for _, name := range names {
		tool, _ := reg.Get(name)
		ts, installed := st.GetTool(name)

		versionStr := "-"
		statusStr := "not installed"

		if installed {
			versionStr = ts.Version
			statusStr = "installed"
		}

		rows = append(rows, output.TableRow{
			Columns: []string{name, versionStr, statusStr, tool.Description},
		})
	}

	output.PrintTable(headers, rows)

	return nil
}
