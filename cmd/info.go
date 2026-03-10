package cmd

import (
	"fmt"
	"os"

	"github.com/sid-technologies/scuta/lib/exitcodes"
	"github.com/sid-technologies/scuta/lib/output"
	"github.com/sid-technologies/scuta/lib/path"
	"github.com/sid-technologies/scuta/lib/registry"
	"github.com/sid-technologies/scuta/lib/state"
	"github.com/sid-technologies/scuta/lib/suggest"

	"github.com/spf13/cobra"
)

func InfoCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "info <tool>",
		Short: "Show detailed information about a tool",
		Long: `Displays comprehensive details about a tool including its description,
repository, install status, version, binary path, and size.`,
		Args: cobra.ExactArgs(1),
		RunE: runInfo,
	}

	return cmd
}

//nolint:gochecknoinits // Standard Cobra pattern
func init() {
	rootCmd.AddCommand(InfoCmd())
}

func runInfo(_ *cobra.Command, args []string) error {
	toolName := args[0]

	reg, err := registry.Load()
	if err != nil {
		return err
	}

	// Validate tool name
	tool, ok := reg.Get(toolName)
	if !ok {
		suggestion := suggest.FormatSuggestion(toolName, reg.Names())
		if suggestion != "" {
			return exitcodes.NewError(exitcodes.InvalidArgs, fmt.Sprintf("unknown tool %q — %s", toolName, suggestion))
		}
		return exitcodes.NewError(exitcodes.InvalidArgs, fmt.Sprintf("unknown tool %q. Run 'scuta list' to see available tools", toolName))
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
	source := reg.Source(toolName)

	// Build info struct
	info := output.ToolInfo{
		Name:        toolName,
		Description: tool.Description,
		Repo:        tool.Repo,
		Source:      source,
		DependsOn:   tool.DependsOn,
	}

	info.Status = "not installed"
	if installed {
		info.Status = "installed"
		info.Version = ts.Version
		info.InstalledAt = ts.InstalledAt.Local().Format("2006-01-02 15:04:05")
		info.BinaryPath = ts.BinaryPath

		if !ts.UpdatedAt.IsZero() {
			info.UpdatedAt = ts.UpdatedAt.Local().Format("2006-01-02 15:04:05")
		}

		// Get binary size
		if ts.BinaryPath != "" {
			if fi, statErr := os.Stat(ts.BinaryPath); statErr == nil {
				info.BinarySize = output.FormatBytes(fi.Size())
			}
		}
	}

	// JSON output
	if output.IsJSON() {
		output.JSON(info)
		return nil
	}

	// Key-value output
	output.PrintKV("Name", toolName)
	output.PrintKV("Description", tool.Description)
	output.PrintKV("Repository", tool.Repo)
	output.PrintKV("Source", source)
	output.PrintKV("Status", info.Status)

	if installed {
		output.PrintKV("Version", ts.Version)
		output.PrintKV("Installed at", info.InstalledAt)
		if info.UpdatedAt != "" {
			output.PrintKV("Updated at", info.UpdatedAt)
		}
		if ts.BinaryPath != "" {
			output.PrintKV("Binary path", ts.BinaryPath)
		}
		if info.BinarySize != "" {
			output.PrintKV("Binary size", info.BinarySize)
		}
	}

	if len(tool.DependsOn) > 0 {
		output.PrintKV("Dependencies", fmt.Sprintf("%v", tool.DependsOn))
	}

	return nil
}
