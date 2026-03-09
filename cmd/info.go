package cmd

import (
	"fmt"
	"os"

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

// toolInfo holds all fields for JSON output.
type toolInfo struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Repository  string   `json:"repository"`
	Source      string   `json:"source"`
	Status      string   `json:"status"`
	Version     string   `json:"version,omitempty"`
	InstalledAt string   `json:"installed_at,omitempty"`
	UpdatedAt   string   `json:"updated_at,omitempty"`
	BinaryPath  string   `json:"binary_path,omitempty"`
	BinarySize  string   `json:"binary_size,omitempty"`
	DependsOn   []string `json:"depends_on,omitempty"`
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
	source := reg.Source(toolName)

	// Build info struct
	info := toolInfo{
		Name:        toolName,
		Description: tool.Description,
		Repository:  tool.Repo,
		Source:      source,
		DependsOn:   tool.DependsOn,
	}

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
				info.BinarySize = formatBytes(fi.Size())
			}
		}
	} else {
		info.Status = "not installed"
	}

	// JSON output
	if output.IsJSON() {
		output.JSON(info)
		return nil
	}

	// Key-value output
	printKV("Name", toolName)
	printKV("Description", tool.Description)
	printKV("Repository", tool.Repo)
	printKV("Source", source)
	printKV("Status", info.Status)

	if installed {
		printKV("Version", ts.Version)
		printKV("Installed at", info.InstalledAt)
		if info.UpdatedAt != "" {
			printKV("Updated at", info.UpdatedAt)
		}
		if ts.BinaryPath != "" {
			printKV("Binary path", ts.BinaryPath)
		}
		if info.BinarySize != "" {
			printKV("Binary size", info.BinarySize)
		}
	}

	if len(tool.DependsOn) > 0 {
		printKV("Dependencies", fmt.Sprintf("%v", tool.DependsOn))
	}

	return nil
}

// printKV prints a key-value pair with consistent padding.
func printKV(key, value string) {
	fmt.Printf("  %s%-14s%s %s\n", output.Muted, key+":", output.Reset, value)
}

// formatBytes returns a human-readable byte size string.
func formatBytes(bytes int64) string {
	const (
		kb = 1024
		mb = 1024 * kb
		gb = 1024 * mb
	)

	switch {
	case bytes >= gb:
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(gb))
	case bytes >= mb:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(mb))
	case bytes >= kb:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(kb))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
