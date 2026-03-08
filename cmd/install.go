package cmd

import (
	"sort"
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

	"github.com/spf13/cobra"
)

var installCmd = &cobra.Command{
	Use:   "install <tool>",
	Short: "Install a tool from the registry",
	Long: `Downloads the correct binary for your OS/architecture from the tool's
GitHub Releases, verifies checksum, and places it in ~/.scuta/bin/.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runInstall,
}

//nolint:gochecknoinits // Standard Cobra pattern
func init() {
	installCmd.Flags().Bool("all", false, "Install all tools from registry")
	installCmd.Flags().String("version", "", "Install a specific version (default: latest)")
	installCmd.Flags().Bool("force", false, "Reinstall even if already installed")
	rootCmd.AddCommand(installCmd)
}

func runInstall(cmd *cobra.Command, args []string) error {
	allFlag, _ := cmd.Flags().GetBool("all")
	versionFlag, _ := cmd.Flags().GetString("version")
	forceFlag, _ := cmd.Flags().GetBool("force")

	reg, err := registry.Load()
	if err != nil {
		return err
	}

	// Determine which tools to install
	var toolNames []string
	if allFlag {
		toolNames = reg.Names()
		sort.Strings(toolNames)
	} else if len(args) == 1 {
		toolName := args[0]
		if _, ok := reg.Get(toolName); !ok {
			suggestion := suggest.FormatSuggestion(toolName, reg.Names())
			if suggestion != "" {
				output.Error("unknown tool %q — %s", toolName, suggestion)
			} else {
				output.Error("unknown tool %q. Run 'scuta list' to see available tools", toolName)
			}
			return nil
		}
		toolNames = []string{toolName}
	} else {
		output.Error("specify a tool name or use --all")
		return nil
	}

	scutaDir, err := path.ScutaDir()
	if err != nil {
		return err
	}

	// Acquire install lock
	if err := lock.Acquire(scutaDir, "install", toolNames, forceFlag); err != nil {
		return err
	}
	defer lock.Release(scutaDir)

	st, err := state.Load(scutaDir)
	if err != nil {
		return err
	}

	token := auth.ResolveTokenWithConfig(scutaDir)
	ghClient := github.NewClient(token)
	inst := installer.New(ghClient, scutaDir)

	start := time.Now()
	var toolResults []history.ToolResult
	successCount := 0

	for _, toolName := range toolNames {
		tool, _ := reg.Get(toolName)
		toolStart := time.Now()

		// Check if already installed at same version (skip unless force)
		if !forceFlag && versionFlag == "" {
			if ts, installed := st.GetTool(toolName); installed {
				output.Info("%s %s already installed (use --force to reinstall)", toolName, ts.Version)
				toolResults = append(toolResults, history.ToolResult{
					Name:     toolName,
					Action:   "install",
					Version:  ts.Version,
					Success:  true,
					Duration: time.Since(toolStart).Round(time.Millisecond).String(),
				})
				successCount++
				continue
			}
		}

		output.Info("Installing %s...", toolName)

		result, err := inst.Install(toolName, tool.Repo, versionFlag, forceFlag)
		if err != nil {
			output.Error("Failed to install %s: %v", toolName, err)
			toolResults = append(toolResults, history.ToolResult{
				Name:     toolName,
				Action:   "install",
				Success:  false,
				Duration: time.Since(toolStart).Round(time.Millisecond).String(),
				Error:    err.Error(),
			})
			continue
		}

		st.SetTool(toolName, state.ToolState{
			Version:     result.Version,
			InstalledAt: time.Now(),
			BinaryPath:  result.BinaryPath,
		})

		output.Success("Installed %s %s", toolName, result.Version)
		toolResults = append(toolResults, history.ToolResult{
			Name:     toolName,
			Action:   "install",
			Version:  result.Version,
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
	allSuccess := successCount == len(toolNames)
	entry := history.NewEntry("install", allSuccess, time.Since(start), toolResults)
	if err := history.Record(scutaDir, entry); err != nil {
		output.Debugf("Failed to record history: %v", err)
	}

	// Print summary
	if len(toolNames) > 1 {
		output.Info("%d/%d tools installed successfully", successCount, len(toolNames))
	}

	return nil
}
