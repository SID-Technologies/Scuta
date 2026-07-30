package cmd

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/sid-technologies/scuta/lib/auth"
	"github.com/sid-technologies/scuta/lib/config"
	"github.com/sid-technologies/scuta/lib/errors"
	"github.com/sid-technologies/scuta/lib/exitcodes"
	"github.com/sid-technologies/scuta/lib/helper"
	"github.com/sid-technologies/scuta/lib/history"
	"github.com/sid-technologies/scuta/lib/installer"
	"github.com/sid-technologies/scuta/lib/manifest"
	"github.com/sid-technologies/scuta/lib/output"
	"github.com/sid-technologies/scuta/lib/path"
	"github.com/sid-technologies/scuta/lib/registry"
	"github.com/sid-technologies/scuta/lib/state"

	"github.com/spf13/cobra"
)

// BundleCmd groups offline-bundle commands: create, verify, install.
func BundleCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bundle",
		Short: "Create, verify, or install from an offline bundle",
		Long: `Create a portable bundle of tools for offline/air-gapped environments.

  scuta bundle create -o scuta-bundle.tar.gz        Bundle all registry tools
  scuta bundle create fzf rg -o tools.tar.gz        Bundle specific tools
  scuta bundle create --from-manifest scuta.lock.yaml   Bundle a manifest's pinned tools
  scuta bundle create --platforms darwin/arm64,linux/amd64   Multi-platform bundle
  scuta bundle create --sign signing.key            Sign the bundle manifest
  scuta bundle verify scuta-bundle.tar.gz           Verify signature + checksums
  scuta bundle install scuta-bundle.tar.gz          Install tools from a bundle`,
	}

	cmd.AddCommand(bundleCreateCmd())
	cmd.AddCommand(bundleVerifyCmd())
	cmd.AddCommand(bundleInstallCmd())

	return cmd
}

func bundleCreateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create [tool...]",
		Short: "Download tools and package them into a bundle",
		Long: `Download tools and package them into a single tar.gz bundle.

By default all registry tools are bundled at their latest versions. Pass
tool names to bundle a subset, or --from-manifest to bundle exactly the
tools and pinned versions of a scuta.lock.yaml.

--platforms builds a bundle for other machines (e.g. create on a connected
macOS laptop, install on air-gapped linux/amd64 servers).

--sign embeds a detached signature of the bundle manifest; since the
manifest pins every asset by SHA-256, the signature covers the full bundle
contents. Verify with: scuta bundle verify`,
		RunE: runBundleCreate,
	}

	cmd.Flags().StringP("output", "o", "scuta-bundle.tar.gz", "Output file path")
	cmd.Flags().String("from-manifest", "", "Bundle the tools (and pinned versions) of a scuta.lock.yaml")
	cmd.Flags().String("platforms", "", "Comma-separated target platforms, e.g. darwin/arm64,linux/amd64 (default: this machine)")
	cmd.Flags().String("sign", "", "Path to a private key (PEM) to sign the bundle manifest")

	return cmd
}

func bundleVerifyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "verify <bundle-path>",
		Short: "Verify a bundle's signature and asset checksums",
		Args:  cobra.ExactArgs(1),
		RunE:  runBundleVerify,
	}

	cmd.Flags().String("key", "", "Path to a public key (PEM); default: signature_public_key from config")

	return cmd
}

func bundleInstallCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "install <bundle-path>",
		Short: "Install tools from a bundle",
		Args:  cobra.ExactArgs(1),
		RunE:  runBundleInstall,
	}

	cmd.Flags().Bool("skip-verify", false, "Skip signature and checksum verification")

	return cmd
}

//nolint:gochecknoinits // Standard Cobra pattern
func init() {
	rootCmd.AddCommand(BundleCmd())
}

// parsePlatforms parses "darwin/arm64,linux/amd64" into platform pairs.
func parsePlatforms(spec string) ([]installer.BundlePlatform, error) {
	if spec == "" {
		return nil, nil
	}

	var platforms []installer.BundlePlatform
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		fields := strings.Split(part, "/")
		if len(fields) != 2 || fields[0] == "" || fields[1] == "" {
			return nil, errors.New("invalid platform %q (expected <os>/<arch>, e.g. linux/amd64)", part)
		}
		platforms = append(platforms, installer.BundlePlatform{OS: fields[0], Arch: fields[1]})
	}
	if len(platforms) == 0 {
		return nil, errors.New("no platforms given")
	}
	return platforms, nil
}

// bundleToolSpecs resolves what to bundle: a manifest's pinned tools, a
// subset of registry tools, or every registry tool.
func bundleToolSpecs(manifestPath string, toolNames []string) (map[string]installer.BundleSpec, error) {
	if manifestPath != "" {
		if len(toolNames) > 0 {
			return nil, errors.New("pass tool names or --from-manifest, not both")
		}

		man, err := manifest.Load(manifestPath)
		if err != nil {
			return nil, err
		}

		reg, regErr := registry.Load()
		specs := make(map[string]installer.BundleSpec, len(man.Tools))
		for name, entry := range man.Tools {
			repo := entry.Repo
			if repo == "" && regErr == nil {
				if tool, ok := reg.Get(name); ok {
					repo = tool.Repo
				}
			}
			if repo == "" {
				return nil, errors.New("manifest entry %q has no repo and is not in the registry", name)
			}
			specs[name] = installer.BundleSpec{Repo: repo, Version: entry.Version}
		}
		return specs, nil
	}

	reg, err := registry.Load()
	if err != nil {
		return nil, err
	}

	names := toolNames
	if len(names) == 0 {
		names = reg.Names()
	}

	specs := make(map[string]installer.BundleSpec, len(names))
	for _, name := range names {
		tool, ok := reg.Get(name)
		if !ok {
			return nil, errors.New("tool %q not found in registry", name)
		}
		specs[name] = installer.BundleSpec{Repo: tool.Repo}
	}
	return specs, nil
}

func runBundleCreate(cmd *cobra.Command, args []string) error {
	ctx, cleanup := helper.WithSignalCancel(cmd.Context())
	defer cleanup()

	outputPath, _ := cmd.Flags().GetString("output")
	manifestPath, _ := cmd.Flags().GetString("from-manifest")
	platformsSpec, _ := cmd.Flags().GetString("platforms")
	signKeyPath, _ := cmd.Flags().GetString("sign")

	specs, err := bundleToolSpecs(manifestPath, args)
	if err != nil {
		return err
	}

	platforms, err := parsePlatforms(platformsSpec)
	if err != nil {
		return err
	}

	var signKey []byte
	if signKeyPath != "" {
		signKey, err = os.ReadFile(signKeyPath) //nolint:gosec // user-supplied path by design
		if err != nil {
			return errors.Wrap(err, "reading signing key")
		}
	}

	scutaDir, err := path.ScutaDir()
	if err != nil {
		return err
	}

	token := auth.ResolveTokenWithConfig(scutaDir)
	ghClient := newGitHubClient(token, scutaDir)

	output.Header("Creating bundle")

	man, err := installer.CreateBundle(ctx, ghClient, installer.CreateBundleOpts{
		Tools:         specs,
		Platforms:     platforms,
		SigningKeyPEM: signKey,
	}, outputPath)
	if err != nil {
		return exitcodes.NewError(exitcodes.Install, fmt.Sprintf("bundle creation failed: %v", err))
	}

	info, _ := os.Stat(outputPath)
	sizeMB := float64(info.Size()) / 1024 / 1024

	fmt.Println()
	signedNote := ""
	if signKeyPath != "" {
		signedNote = ", signed"
	}
	output.Success("Bundle created: %s (%.1f MB, %d tools, %d platform(s)%s)",
		outputPath, sizeMB, len(man.Tools), len(man.Platforms), signedNote)
	output.Dimmed("  Transfer to air-gapped machine and run: scuta bundle install %s", outputPath)

	return nil
}

func runBundleVerify(cmd *cobra.Command, args []string) error {
	bundlePath := args[0]
	keyPath, _ := cmd.Flags().GetString("key")

	if _, err := os.Stat(bundlePath); err != nil {
		return exitcodes.NewError(exitcodes.IO, fmt.Sprintf("bundle file not found: %s", bundlePath))
	}

	scutaDir, err := path.ScutaDir()
	if err != nil {
		return err
	}

	var pubKey []byte
	if keyPath != "" {
		pubKey, err = os.ReadFile(keyPath) //nolint:gosec // user-supplied path by design
		if err != nil {
			return errors.Wrap(err, "reading public key")
		}
	} else if cfg := config.LoadTrusted(scutaDir); cfg.SignaturePublicKey != "" {
		pubKey = []byte(cfg.SignaturePublicKey)
	}

	man, tmpDir, err := installer.ExtractBundle(bundlePath)
	if err != nil {
		return exitcodes.NewError(exitcodes.Install, fmt.Sprintf("failed to extract bundle: %v", err))
	}
	defer os.RemoveAll(tmpDir)

	failed := 0

	signed, sigErr := installer.VerifyBundleSignature(tmpDir, pubKey, false)
	switch {
	case sigErr != nil:
		output.Error("Signature: %v", sigErr)
		failed++
	case signed:
		output.Success("Signature: valid")
	default:
		output.Warning("Signature: bundle is not signed")
	}

	for _, res := range installer.VerifyBundleChecksums(tmpDir, man) {
		if res.Err != nil {
			output.Error("%s (%s): %v", res.Tool, res.Platform, res.Err)
			failed++
			continue
		}
		output.Success("%s (%s): checksum OK", res.Tool, res.Platform)
	}

	if failed > 0 {
		return exitcodes.NewError(exitcodes.Install, fmt.Sprintf("bundle verification failed: %d problem(s)", failed))
	}

	output.Info("Bundle OK: %d tools, %d platform(s)", len(man.Tools), maxInt(len(man.Platforms), 1))
	return nil
}

// maxInt avoids pulling in generics helpers for one comparison.
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func runBundleInstall(cmd *cobra.Command, args []string) error {
	ctx, cleanup := helper.WithSignalCancel(cmd.Context())
	defer cleanup()

	bundlePath := args[0]
	skipVerify, _ := cmd.Flags().GetBool("skip-verify")

	// Validate bundle exists
	if _, err := os.Stat(bundlePath); err != nil {
		return exitcodes.NewError(exitcodes.IO, fmt.Sprintf("bundle file not found: %s", bundlePath))
	}

	output.Info("Extracting bundle: %s", bundlePath)

	man, tmpDir, err := installer.ExtractBundle(bundlePath)
	if err != nil {
		return exitcodes.NewError(exitcodes.Install, fmt.Sprintf("failed to extract bundle: %v", err))
	}
	defer os.RemoveAll(tmpDir)

	scutaDir, err := path.ScutaDir()
	if err != nil {
		return err
	}

	// Bundle signature check. The trust root only ever comes from local or
	// system config (LoadTrusted). An invalid signature is always fatal; a
	// missing one is fatal only with require_signature enabled.
	if skipVerify {
		output.Warning("Skipping bundle verification (--skip-verify)")
	} else {
		cfg := config.LoadTrusted(scutaDir)
		signed, sigErr := installer.VerifyBundleSignature(tmpDir, []byte(cfg.SignaturePublicKey), cfg.RequireSignature)
		if sigErr != nil {
			return exitcodes.NewError(exitcodes.Install, fmt.Sprintf("bundle verification failed: %v", sigErr))
		}
		if signed {
			output.Success("Bundle signature verified")
		}
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

	for name, info := range man.Tools {
		if ctx.Err() != nil {
			break
		}

		toolStart := time.Now()

		result, err := installBundleTool(inst, tmpDir, man, name, skipVerify)
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
	allSuccess := successCount == len(man.Tools)
	entry := history.NewEntry("bundle-install", allSuccess, time.Since(start), toolResults)
	if err := history.Record(scutaDir, entry); err != nil {
		output.Debugf("Failed to record history: %v", err)
	}

	output.Info("%d/%d tools installed from bundle", successCount, len(man.Tools))
	if !allSuccess {
		return exitcodes.NewError(exitcodes.Install, "some tools failed to install from the bundle")
	}
	return nil
}

// installBundleTool verifies and installs a single tool from an extracted
// bundle, resolving the right asset for this machine's platform.
func installBundleTool(inst *installer.Installer, tmpDir string, man *installer.BundleManifest, name string, skipVerify bool) (*installer.InstallResult, error) {
	relPath, checksum, err := installer.BundleAssetForHost(man, name)
	if err != nil {
		return nil, err
	}

	assetPath := fmt.Sprintf("%s/%s", tmpDir, relPath)
	if !skipVerify && checksum != "" {
		if err := installer.VerifyChecksum(assetPath, checksum); err != nil {
			return nil, err
		}
	}

	return inst.InstallFromArchive(name, assetPath)
}
