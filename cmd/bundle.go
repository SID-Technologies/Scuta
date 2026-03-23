package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/sid-technologies/scuta/lib/auth"
	"github.com/sid-technologies/scuta/lib/exitcodes"
	"github.com/sid-technologies/scuta/lib/helper"
	"github.com/sid-technologies/scuta/lib/history"
	"github.com/sid-technologies/scuta/lib/installer"
	"github.com/sid-technologies/scuta/lib/output"
	"github.com/sid-technologies/scuta/lib/path"
	"github.com/sid-technologies/scuta/lib/registry"
	"github.com/sid-technologies/scuta/lib/state"

	"github.com/spf13/cobra"
)

func BundleCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bundle",
		Short: "Create or install from an offline bundle",
		Long: `Create a portable bundle of all registry tools for offline/air-gapped environments.

  scuta bundle create -o scuta-bundle.tar.gz       Create a bundle with all tools
  scuta bundle install scuta-bundle.tar.gz          Install tools from a bundle`,
	}

	cmd.AddCommand(bundleCreateCmd())
	cmd.AddCommand(bundleInstallCmd())

	return cmd
}

func bundleCreateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Download all tools and package into a bundle",
		RunE:  runBundleCreate,
	}

	cmd.Flags().StringP("output", "o", "scuta-bundle.tar.gz", "Output file path")

	return cmd
}

func bundleInstallCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "install <bundle-path>",
		Short: "Install tools from a bundle",
		Args:  cobra.ExactArgs(1),
		RunE:  runBundleInstall,
	}

	return cmd
}

//nolint:gochecknoinits // Standard Cobra pattern
func init() {
	rootCmd.AddCommand(BundleCmd())
}

func runBundleCreate(cmd *cobra.Command, _ []string) error {
	ctx, cleanup := helper.WithSignalCancel(cmd.Context())
	defer cleanup()

	outputPath, _ := cmd.Flags().GetString("output")

	reg, err := registry.Load()
	if err != nil {
		return err
	}

	scutaDir, err := path.ScutaDir()
	if err != nil {
		return err
	}

	token := auth.ResolveTokenWithConfig(scutaDir)
	ghClient := newGitHubClient(token, scutaDir)

	// Build tools map from registry
	tools := make(map[string]string)
	for _, name := range reg.Names() {
		tool, _ := reg.Get(name)
		tools[name] = tool.Repo
	}

	output.Header("Creating bundle")

	manifest, err := installer.CreateBundle(ctx, ghClient, tools, outputPath)
	if err != nil {
		return exitcodes.NewError(exitcodes.Install, fmt.Sprintf("bundle creation failed: %v", err))
	}

	info, _ := os.Stat(outputPath)
	sizeMB := float64(info.Size()) / 1024 / 1024

	fmt.Println()
	output.Success("Bundle created: %s (%.1f MB, %d tools)", outputPath, sizeMB, len(manifest.Tools))
	output.Dimmed("  Transfer to air-gapped machine and run: scuta bundle install %s", outputPath)

	return nil
}

func runBundleInstall(cmd *cobra.Command, args []string) error {
	ctx, cleanup := helper.WithSignalCancel(cmd.Context())
	defer cleanup()

	bundlePath := args[0]

	// Validate bundle exists
	if _, err := os.Stat(bundlePath); err != nil {
		return exitcodes.NewError(exitcodes.IO, fmt.Sprintf("bundle file not found: %s", bundlePath))
	}

	output.Info("Extracting bundle: %s", bundlePath)

	manifest, tmpDir, err := installer.ExtractBundle(bundlePath)
	if err != nil {
		return exitcodes.NewError(exitcodes.Install, fmt.Sprintf("failed to extract bundle: %v", err))
	}
	defer os.RemoveAll(tmpDir)

	scutaDir, err := path.ScutaDir()
	if err != nil {
		return err
	}

	ghClient := newGitHubClient("", scutaDir)
	inst := installer.New(ghClient, scutaDir)
	st, err := state.Load(scutaDir)
	if err != nil {
		return err
	}

	start := time.Now()
	var toolResults []history.ToolResult
	successCount := 0

	for name, info := range manifest.Tools {
		if ctx.Err() != nil {
			break
		}

		toolStart := time.Now()

		// Verify checksum if available
		assetPath := fmt.Sprintf("%s/%s", tmpDir, info.Asset)
		if info.Checksum != "" {
			if err := installer.VerifyChecksum(assetPath, info.Checksum); err != nil {
				output.Error("Checksum mismatch for %s: %v", name, err)
				toolResults = append(toolResults, history.ToolResult{
					Name:     name,
					Action:   "install",
					Success:  false,
					Duration: time.Since(toolStart).Round(time.Millisecond).String(),
					Error:    err.Error(),
				})
				continue
			}
		}

		// Install from the asset archive
		result, err := inst.InstallFromArchive(name, assetPath)
		if err != nil {
			output.Error("Failed to install %s: %v", name, err)
			toolResults = append(toolResults, history.ToolResult{
				Name:     name,
				Action:   "install",
				Success:  false,
				Duration: time.Since(toolStart).Round(time.Millisecond).String(),
				Error:    err.Error(),
			})
			continue
		}

		st.SetTool(name, state.ToolState{
			Version:     info.Version,
			InstalledAt: time.Now(),
			BinaryPath:  result.BinaryPath,
		})

		output.Success("Installed %s %s", name, info.Version)
		toolResults = append(toolResults, history.ToolResult{
			Name:     name,
			Action:   "install",
			Version:  info.Version,
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
	allSuccess := successCount == len(manifest.Tools)
	entry := history.NewEntry("bundle-install", allSuccess, time.Since(start), toolResults)
	if err := history.Record(scutaDir, entry); err != nil {
		output.Debugf("Failed to record history: %v", err)
	}

	output.Info("%d/%d tools installed from bundle", successCount, len(manifest.Tools))
	return nil
}
