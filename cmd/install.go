package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/sid-technologies/scuta/lib/auth"
	"github.com/sid-technologies/scuta/lib/config"
	"github.com/sid-technologies/scuta/lib/exitcodes"
	"github.com/sid-technologies/scuta/lib/github"
	"github.com/sid-technologies/scuta/lib/graph"
	"github.com/sid-technologies/scuta/lib/helper"
	"github.com/sid-technologies/scuta/lib/history"
	"github.com/sid-technologies/scuta/lib/installer"
	"github.com/sid-technologies/scuta/lib/lock"
	"github.com/sid-technologies/scuta/lib/output"
	"github.com/sid-technologies/scuta/lib/path"
	"github.com/sid-technologies/scuta/lib/policy"
	"github.com/sid-technologies/scuta/lib/registry"
	"github.com/sid-technologies/scuta/lib/state"
	"github.com/sid-technologies/scuta/lib/suggest"
	"github.com/sid-technologies/scuta/lib/telemetry"
	workerqueue "github.com/sid-technologies/scuta/lib/worker_queue"

	"github.com/spf13/cobra"
)

// isRoot returns true if the current process has root/admin privileges.
func isRoot() bool {
	if runtime.GOOS == "windows" {
		// On Windows, check for admin by attempting to open a protected path.
		// A simple heuristic: try to create a file in the system directory.
		f, err := os.CreateTemp(os.Getenv("SystemRoot"), "scuta-admin-check-*")
		if err != nil {
			return false
		}
		name := f.Name()
		f.Close()
		os.Remove(name)
		return true
	}
	return os.Getuid() == 0
}

func InstallCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "install <tool | owner/repo>",
		Short: "Install a tool from the registry or any GitHub repo",
		Long: `Downloads the correct binary for your OS/architecture from the tool's
GitHub Releases, verifies checksum, and places it in ~/.scuta/bin/.

Use a registry name (e.g., "fzf") or a GitHub owner/repo (e.g., "junegunn/fzf")
to install any tool that publishes GitHub Releases.`,
		Args: cobra.MaximumNArgs(1),
		RunE: runInstall,
	}

	cmd.Flags().Bool("all", false, "Install all tools from registry")
	cmd.Flags().String("version", "", "Install a specific version (default: latest)")
	cmd.Flags().Bool("force", false, "Reinstall even if already installed")
	cmd.Flags().Bool("skip-verify", false, "Skip checksum verification")
	cmd.Flags().Bool("dry-run", false, "Show what would be installed without installing")
	cmd.Flags().String("from", "", "Install from a local archive file (offline/air-gapped)")
	cmd.Flags().Bool("system", false, "Install to system-wide location (requires root/admin)")

	return cmd
}

//nolint:gochecknoinits // Standard Cobra pattern
func init() {
	rootCmd.AddCommand(InstallCmd())
}

func runInstall(cmd *cobra.Command, args []string) error {
	ctx, cleanup := helper.WithSignalCancel(cmd.Context())
	defer cleanup()

	allFlag, _ := cmd.Flags().GetBool("all")
	versionFlag, _ := cmd.Flags().GetString("version")
	forceFlag, _ := cmd.Flags().GetBool("force")
	skipVerifyFlag, _ := cmd.Flags().GetBool("skip-verify")
	dryRunFlag, _ := cmd.Flags().GetBool("dry-run")
	fromFlag, _ := cmd.Flags().GetString("from")
	systemFlag, _ := cmd.Flags().GetBool("system")

	// System-wide install requires root/admin
	if systemFlag {
		if !isRoot() {
			return exitcodes.NewError(exitcodes.General, "system-wide install requires root/admin privileges (try sudo)")
		}
	}

	// Offline install from local archive
	if fromFlag != "" {
		return runInstallFromArchive(ctx, args, fromFlag)
	}

	// Direct install from owner/repo (e.g., "scuta install junegunn/fzf")
	if !allFlag && len(args) == 1 && strings.Contains(args[0], "/") {
		return runInstallDirect(ctx, cmd, args[0], versionFlag, forceFlag, skipVerifyFlag, systemFlag)
	}

	reg, err := registry.Load()
	if err != nil {
		return err
	}

	// Determine which tools to install
	var toolNames []string
	if allFlag {
		toolNames = reg.Names()
	} else if len(args) == 1 {
		toolName := args[0]
		if _, ok := reg.Get(toolName); !ok {
			suggestion := suggest.FormatSuggestion(toolName, reg.Names())
			if suggestion != "" {
				return exitcodes.NewError(exitcodes.InvalidArgs, fmt.Sprintf("unknown tool %q — %s", toolName, suggestion))
			}
			return exitcodes.NewError(exitcodes.InvalidArgs, fmt.Sprintf("unknown tool %q. Run 'scuta list' to see available tools", toolName))
		}
		toolNames = []string{toolName}
	} else {
		return exitcodes.NewError(exitcodes.InvalidArgs, "specify a tool name, owner/repo, or use --all")
	}

	// Sort by dependency order using the graph
	toolNames = orderByDependencies(toolNames, reg)

	scutaDir, err := path.ScutaDir()
	if err != nil {
		return err
	}

	// Load policy (best-effort)
	pol := loadPolicy(scutaDir)

	// Acquire install lock
	if err := lock.Acquire(scutaDir, "install", toolNames, forceFlag); err != nil {
		return err
	}
	defer lock.Release(scutaDir)

	// For system installs, use system state and system bin dir
	var st *state.State
	var stateDir string
	var inst *installer.Installer

	token := auth.ResolveTokenWithConfig(scutaDir)
	ghClient := newGitHubClient(token, scutaDir)

	if systemFlag {
		stateDir = path.SystemStateDir()
		st, err = state.Load(stateDir)
		if err != nil {
			return err
		}
		inst = installer.NewWithBinDir(ghClient, scutaDir, path.SystemBinDir())
		output.Info("Installing to system-wide location: %s", path.SystemBinDir())
	} else {
		stateDir = scutaDir
		st, err = state.Load(scutaDir)
		if err != nil {
			return err
		}
		inst = installer.New(ghClient, scutaDir)
	}

	// Configure signature verification from config
	cfg, cfgLoadErr := config.Load(scutaDir)
	if cfgLoadErr == nil && (cfg.RequireSignature || cfg.SignaturePublicKey != "") {
		inst.SetSignatureVerification(cfg.RequireSignature, []byte(cfg.SignaturePublicKey))
	}

	start := time.Now()

	// Use parallel installs for --all with multiple tools, sequential otherwise
	var toolResults []history.ToolResult
	var successCount int

	if dryRunFlag {
		toolResults, successCount = installDryRun(ctx, toolNames, reg, ghClient, versionFlag)
	} else if allFlag && len(toolNames) > 1 {
		toolResults, successCount = installParallel(ctx, toolNames, reg, inst, st, pol, versionFlag, forceFlag, skipVerifyFlag)
	} else {
		toolResults, successCount = installSequential(ctx, toolNames, reg, inst, st, pol, versionFlag, forceFlag, skipVerifyFlag)
	}

	// Skip state save and history if canceled or dry-run
	if ctx.Err() != nil || dryRunFlag {
		return nil
	}

	// Save state (to system dir for --system, user dir otherwise)
	if err := st.Save(stateDir); err != nil {
		output.Error("Failed to save state: %v", err)
	}

	// Record history
	allSuccess := successCount == len(toolNames)
	entry := history.NewEntry("install", allSuccess, time.Since(start), toolResults)
	if err := history.Record(scutaDir, entry); err != nil {
		output.Debugf("Failed to record history: %v", err)
	}

	// Telemetry (best-effort, uses merged config for effective settings)
	mergedCfg, mergedErr := config.LoadWithMerge(scutaDir)
	if mergedErr == nil {
		_ = telemetry.Record(scutaDir, mergedCfg.Telemetry, "install")
	}

	// Print summary
	if len(toolNames) > 1 {
		output.Info("%d/%d tools installed successfully", successCount, len(toolNames))
	}

	return nil
}

// installDryRun prints what would be installed without actually installing.
func installDryRun(
	ctx context.Context,
	toolNames []string,
	reg *registry.Registry,
	ghClient *github.Client,
	versionFlag string,
) ([]history.ToolResult, int) {
	var toolResults []history.ToolResult
	successCount := 0

	for _, toolName := range toolNames {
		if ctx.Err() != nil {
			break
		}

		tool, _ := reg.Get(toolName)
		toolStart := time.Now()

		var version string
		if versionFlag != "" {
			version = versionFlag
		} else {
			release, err := ghClient.GetLatestRelease(ctx, tool.Repo)
			if err != nil {
				output.Error("[dry run] Failed to fetch release for %s: %v", toolName, err)
				toolResults = append(toolResults, history.ToolResult{
					Name:     toolName,
					Action:   "install",
					Success:  false,
					Duration: time.Since(toolStart).Round(time.Millisecond).String(),
					Error:    err.Error(),
				})
				continue
			}
			version = github.NormalizeVersion(release.TagName)
		}

		output.Info("[dry run] Would install %s %s", toolName, version)
		toolResults = append(toolResults, history.ToolResult{
			Name:     toolName,
			Action:   "install",
			Version:  version,
			Success:  true,
			Duration: time.Since(toolStart).Round(time.Millisecond).String(),
		})
		successCount++
	}

	return toolResults, successCount
}

// installSequential installs tools one at a time.
func installSequential(
	ctx context.Context,
	toolNames []string,
	reg *registry.Registry,
	inst *installer.Installer,
	st *state.State,
	pol *policy.Policy,
	versionFlag string,
	forceFlag bool,
	skipVerifyFlag bool,
) ([]history.ToolResult, int) {
	var toolResults []history.ToolResult
	successCount := 0

	for _, toolName := range toolNames {
		if ctx.Err() != nil {
			break
		}
		result := installOneTool(ctx, toolName, reg, inst, st, pol, versionFlag, forceFlag, skipVerifyFlag)
		toolResults = append(toolResults, result)
		if result.Success {
			successCount++
		}
	}

	return toolResults, successCount
}

// installParallel installs tools concurrently using a worker queue.
// Tools at the same dependency depth run in parallel; deeper levels wait.
func installParallel(
	ctx context.Context,
	toolNames []string,
	reg *registry.Registry,
	inst *installer.Installer,
	st *state.State,
	pol *policy.Policy,
	versionFlag string,
	forceFlag bool,
	skipVerifyFlag bool,
) ([]history.ToolResult, int) {
	// Group tools by dependency depth
	depths := calculateDepths(toolNames, reg)
	groups := groupByDepth(toolNames, depths)

	var mu sync.Mutex
	var allResults []history.ToolResult
	totalSuccess := 0

	// Install each depth level in parallel, but wait between levels
	for _, group := range groups {
		if ctx.Err() != nil {
			break
		}

		if len(group) == 1 {
			// Single tool — no need for worker queue overhead
			result := installOneTool(ctx, group[0], reg, inst, st, pol, versionFlag, forceFlag, skipVerifyFlag)
			allResults = append(allResults, result)
			if result.Success {
				totalSuccess++
			}
			continue
		}

		// Multiple tools at this depth — run in parallel
		wq := workerqueue.NewWorkQueue(func(task *workerqueue.TaskInfo) bool {
			result := installOneTool(ctx, task.ToolName, reg, inst, st, pol, versionFlag, forceFlag, skipVerifyFlag)
			mu.Lock()
			allResults = append(allResults, result)
			if result.Success {
				totalSuccess++
			}
			mu.Unlock()
			return result.Success
		}, 0) // 0 = use default (NumCPU/2)

		for _, toolName := range group {
			wq.AddTask(workerqueue.NewTaskInfo(toolName, versionFlag, "install"))
		}

		wq.Execute()
	}

	return allResults, totalSuccess
}

// installOneTool installs a single tool and returns the result.
func installOneTool(
	ctx context.Context,
	toolName string,
	reg *registry.Registry,
	inst *installer.Installer,
	st *state.State,
	pol *policy.Policy,
	versionFlag string,
	forceFlag bool,
	skipVerifyFlag bool,
) history.ToolResult {
	tool, _ := reg.Get(toolName)
	toolStart := time.Now()

	// Check if already installed (skip unless force)
	if !forceFlag && versionFlag == "" {
		if ts, installed := st.GetTool(toolName); installed {
			output.Info("%s %s already installed (use --force to reinstall)", toolName, ts.Version)
			return history.ToolResult{
				Name:     toolName,
				Action:   "install",
				Version:  ts.Version,
				Success:  true,
				Duration: time.Since(toolStart).Round(time.Millisecond).String(),
			}
		}
	}

	// Policy check for pinned version
	if versionFlag != "" {
		if v := pol.CheckToolVersion(toolName, versionFlag); v != nil {
			output.Error("Policy violation for %s: %s", toolName, v.Message)
			return history.ToolResult{
				Name:     toolName,
				Action:   "install",
				Success:  false,
				Duration: time.Since(toolStart).Round(time.Millisecond).String(),
				Error:    v.Message,
			}
		}
	}

	output.Info("Installing %s...", toolName)

	// Use InstallWithOpts when tool has extended options, otherwise use standard Install
	var result *installer.InstallResult
	var err error

	if hasExtendedOpts(tool) {
		opts := buildInstallOpts(tool)
		result, err = inst.InstallWithOpts(ctx, toolName, tool.Repo, versionFlag, forceFlag, skipVerifyFlag, opts)
	} else {
		result, err = inst.Install(ctx, toolName, tool.Repo, versionFlag, forceFlag, skipVerifyFlag)
	}
	if err != nil {
		output.Error("Failed to install %s: %v", toolName, err)
		return history.ToolResult{
			Name:     toolName,
			Action:   "install",
			Success:  false,
			Duration: time.Since(toolStart).Round(time.Millisecond).String(),
			Error:    err.Error(),
		}
	}

	// Policy check for resolved version (when no specific version was requested)
	if versionFlag == "" {
		if v := pol.CheckToolVersion(toolName, result.Version); v != nil {
			output.Error("Policy violation for %s: %s", toolName, v.Message)
			// Remove the just-installed binary — use effective bin name
			uninstallName := toolName
			if tool.Bin != "" {
				uninstallName = tool.Bin
			}
			_ = inst.Uninstall(uninstallName)
			return history.ToolResult{
				Name:     toolName,
				Action:   "install",
				Success:  false,
				Duration: time.Since(toolStart).Round(time.Millisecond).String(),
				Error:    v.Message,
			}
		}
	}

	st.SetTool(toolName, state.ToolState{
		Version:     result.Version,
		InstalledAt: time.Now(),
		BinaryPath:  result.BinaryPath,
	})

	output.Success("Installed %s %s", toolName, result.Version)
	return history.ToolResult{
		Name:     toolName,
		Action:   "install",
		Version:  result.Version,
		Success:  true,
		Duration: time.Since(toolStart).Round(time.Millisecond).String(),
	}
}

// hasExtendedOpts returns true if the tool definition has any extended asset options.
func hasExtendedOpts(tool registry.Tool) bool {
	return tool.Asset != "" || tool.Bin != "" || len(tool.OSMap) > 0 || len(tool.ArchMap) > 0 || tool.VersionPrefix != ""
}

// buildInstallOpts creates InstallOpts from a registry Tool definition.
func buildInstallOpts(tool registry.Tool) installer.InstallOpts {
	return installer.InstallOpts{
		AssetTemplate: tool.Asset,
		BinName:       tool.Bin,
		OSMap:         tool.OSMap,
		ArchMap:       tool.ArchMap,
		VersionPrefix: tool.VersionPrefix,
	}
}

// orderByDependencies sorts tool names so dependencies come first.
// Falls back to the original order if there are no dependencies or errors.
func orderByDependencies(toolNames []string, reg *registry.Registry) []string {
	g := graph.New()

	// Build graph from registry dependencies
	nameSet := make(map[string]bool, len(toolNames))
	for _, name := range toolNames {
		nameSet[name] = true
	}

	for _, name := range toolNames {
		tool, _ := reg.Get(name)
		// Only include dependencies that are in our install set
		var deps []string
		for _, dep := range tool.DependsOn {
			if nameSet[dep] {
				deps = append(deps, dep)
			}
		}
		g.AddNode(name, deps)
	}

	if err := g.ValidateDependencies(); err != nil {
		output.Debugf("Dependency validation failed, using default order: %v", err)
		return toolNames
	}

	sorted, err := g.TopologicalSort()
	if err != nil {
		output.Debugf("Topological sort failed, using default order: %v", err)
		return toolNames
	}

	return sorted
}

// calculateDepths returns the dependency depth of each tool.
func calculateDepths(toolNames []string, reg *registry.Registry) map[string]int {
	g := graph.New()

	nameSet := make(map[string]bool, len(toolNames))
	for _, name := range toolNames {
		nameSet[name] = true
	}

	for _, name := range toolNames {
		tool, _ := reg.Get(name)
		var deps []string
		for _, dep := range tool.DependsOn {
			if nameSet[dep] {
				deps = append(deps, dep)
			}
		}
		g.AddNode(name, deps)
	}

	depths, err := g.CalculateDepths()
	if err != nil {
		output.Debugf("Depth calculation failed: %v", err)
		// Return all at depth 0 (everything parallel)
		result := make(map[string]int, len(toolNames))
		for _, name := range toolNames {
			result[name] = 0
		}
		return result
	}

	return depths
}

// groupByDepth groups tool names by their dependency depth for parallel execution.
// Returns groups in order: depth 0 first, then depth 1, etc.
func groupByDepth(toolNames []string, depths map[string]int) [][]string {
	maxDepth := 0
	for _, d := range depths {
		if d > maxDepth {
			maxDepth = d
		}
	}

	groups := make([][]string, maxDepth+1)
	for _, name := range toolNames {
		d := depths[name]
		groups[d] = append(groups[d], name)
	}

	// Filter out empty groups
	var result [][]string
	for _, group := range groups {
		if len(group) > 0 {
			result = append(result, group)
		}
	}

	return result
}

// runInstallFromArchive handles offline installation from a local archive file.
func runInstallFromArchive(ctx context.Context, args []string, archivePath string) error {
	if len(args) != 1 {
		return exitcodes.NewError(exitcodes.InvalidArgs, "specify a tool name when using --from (e.g., scuta install pilum --from ./archive.tar.gz)")
	}

	toolName := args[0]

	// Validate archive file exists
	if _, err := os.Stat(archivePath); err != nil {
		return exitcodes.NewError(exitcodes.IO, fmt.Sprintf("archive file not found: %s", archivePath))
	}

	scutaDir, err := path.ScutaDir()
	if err != nil {
		return err
	}

	ghClient := github.NewClient("")
	inst := installer.New(ghClient, scutaDir)

	start := time.Now()
	output.Info("Installing %s from %s...", toolName, archivePath)

	result, err := inst.InstallFromArchive(toolName, archivePath)
	if err != nil {
		return exitcodes.NewError(exitcodes.Install, fmt.Sprintf("failed to install %s: %v", toolName, err))
	}

	// Update state
	st, err := state.Load(scutaDir)
	if err != nil {
		return err
	}

	st.SetTool(toolName, state.ToolState{
		Version:     result.Version,
		InstalledAt: time.Now(),
		BinaryPath:  result.BinaryPath,
	})

	if err := st.Save(scutaDir); err != nil {
		output.Error("Failed to save state: %v", err)
	}

	// Record history
	entry := history.NewEntry("install", true, time.Since(start), []history.ToolResult{
		{
			Name:     toolName,
			Action:   "install",
			Version:  result.Version,
			Success:  true,
			Duration: time.Since(start).Round(time.Millisecond).String(),
		},
	})
	if err := history.Record(scutaDir, entry); err != nil {
		output.Debugf("Failed to record history: %v", err)
	}

	_ = ctx // context not needed for local install but kept for signature consistency

	output.Success("Installed %s %s (from local archive)", toolName, result.Version)
	return nil
}

// runInstallDirect installs a tool directly from a GitHub repo (owner/repo format).
// The tool does not need to be in the registry. Asset matching uses heuristics.
func runInstallDirect(ctx context.Context, cmd *cobra.Command, repoArg string, versionFlag string, forceFlag bool, skipVerifyFlag bool, systemFlag bool) error {
	parts := strings.SplitN(repoArg, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return exitcodes.NewError(exitcodes.InvalidArgs, fmt.Sprintf("invalid repo format %q — expected owner/repo", repoArg))
	}

	repo := repoArg
	toolName := strings.ToLower(parts[1])

	scutaDir, err := path.ScutaDir()
	if err != nil {
		return err
	}

	token := auth.ResolveTokenWithConfig(scutaDir)
	ghClient := newGitHubClient(token, scutaDir)

	// Acquire install lock
	if err := lock.Acquire(scutaDir, "install", []string{toolName}, forceFlag); err != nil {
		return err
	}
	defer lock.Release(scutaDir)

	// Determine state and installer based on --system flag
	var st *state.State
	var stateDir string
	var inst *installer.Installer

	if systemFlag {
		stateDir = path.SystemStateDir()
		st, err = state.Load(stateDir)
		if err != nil {
			return err
		}
		inst = installer.NewWithBinDir(ghClient, scutaDir, path.SystemBinDir())
	} else {
		stateDir = scutaDir
		st, err = state.Load(scutaDir)
		if err != nil {
			return err
		}
		inst = installer.New(ghClient, scutaDir)
	}

	// Check if already installed (skip unless force)
	if !forceFlag && versionFlag == "" {
		if ts, installed := st.GetTool(toolName); installed {
			output.Info("%s %s already installed (use --force to reinstall)", toolName, ts.Version)
			return nil
		}
	}

	output.Info("Installing %s from %s...", toolName, repo)

	// Get the release
	var release *github.Release
	if versionFlag != "" {
		release, err = ghClient.GetRelease(ctx, repo, versionFlag)
	} else {
		release, err = ghClient.GetLatestRelease(ctx, repo)
	}
	if err != nil {
		return exitcodes.NewError(exitcodes.Install, fmt.Sprintf("failed to fetch release for %s: %v", repo, err))
	}

	version := github.NormalizeVersion(release.TagName)

	// Try to find a matching asset using heuristic matching
	asset, err := github.FindAssetHeuristic(release.Assets, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return exitcodes.NewError(exitcodes.Install, fmt.Sprintf("no matching asset found for %s/%s in %s %s: %v", runtime.GOOS, runtime.GOARCH, repo, version, err))
	}

	output.Debugf("Found asset: %s (%d bytes)", asset.Name, asset.Size)

	// Download to temp directory
	tmpDir, err := os.MkdirTemp("", "scuta-direct-*")
	if err != nil {
		return exitcodes.NewError(exitcodes.IO, fmt.Sprintf("creating temp directory: %v", err))
	}
	defer os.RemoveAll(tmpDir)

	archivePath := filepath.Join(tmpDir, asset.Name)
	if err := ghClient.DownloadAsset(ctx, asset.BrowserDownloadURL, archivePath); err != nil {
		return exitcodes.NewError(exitcodes.Install, fmt.Sprintf("downloading %s: %v", asset.Name, err))
	}

	// Best-effort checksum verification for unregistered tools
	if skipVerifyFlag {
		output.Warning("Skipping checksum verification (--skip-verify)")
	} else {
		checksums, csErr := ghClient.DownloadChecksums(ctx, release)
		if csErr != nil {
			output.Warning("Could not download checksums for %s: %v", toolName, csErr)
		} else if checksums == nil {
			output.Warning("No checksums file found for %s — skipping verification", toolName)
		} else {
			expectedHash, ok := checksums[asset.Name]
			if !ok {
				output.Warning("No checksum entry for %s — skipping verification", asset.Name)
			} else {
				if verifyErr := installer.VerifyChecksum(archivePath, expectedHash); verifyErr != nil {
					return exitcodes.NewError(exitcodes.Install, fmt.Sprintf("checksum mismatch for %s: %v", toolName, verifyErr))
				}
				output.Debugf("Checksum verified for %s", asset.Name)
			}
		}
	}

	if ctx.Err() != nil {
		return nil
	}

	// Determine bin directory
	binDir := filepath.Join(scutaDir, "bin")
	if systemFlag {
		binDir = path.SystemBinDir()
	}
	if err := os.MkdirAll(binDir, 0o700); err != nil {
		return exitcodes.NewError(exitcodes.IO, fmt.Sprintf("creating bin directory: %v", err))
	}

	binaryPath := filepath.Join(binDir, installer.BinaryName(toolName))

	// Handle raw binaries vs archives
	if github.IsRawBinary(asset.Name) {
		// Raw binary — copy directly
		tempPath := binaryPath + ".tmp"
		if err := installer.CopyFile(archivePath, tempPath); err != nil {
			os.Remove(tempPath)
			return exitcodes.NewError(exitcodes.Install, fmt.Sprintf("installing binary: %v", err))
		}
		if err := os.Chmod(tempPath, 0o755); err != nil {
			os.Remove(tempPath)
			return exitcodes.NewError(exitcodes.Install, fmt.Sprintf("setting permissions: %v", err))
		}
		if err := os.Rename(tempPath, binaryPath); err != nil {
			os.Remove(tempPath)
			return exitcodes.NewError(exitcodes.Install, fmt.Sprintf("installing binary: %v", err))
		}
	} else {
		// Archive — extract and find binary
		result, installErr := inst.InstallFromArchive(toolName, archivePath)
		if installErr != nil {
			return exitcodes.NewError(exitcodes.Install, fmt.Sprintf("failed to install %s: %v", toolName, installErr))
		}
		binaryPath = result.BinaryPath
	}

	// Update state with repo info
	st.SetTool(toolName, state.ToolState{
		Version:     version,
		InstalledAt: time.Now(),
		BinaryPath:  binaryPath,
		Repo:        repo,
	})

	if err := st.Save(stateDir); err != nil {
		output.Error("Failed to save state: %v", err)
	}

	// Auto-add to local registry so "scuta update" works later
	localReg, err := registry.LoadLocal(scutaDir)
	if err != nil {
		output.Debugf("Failed to load local registry: %v", err)
	} else {
		if _, exists := localReg.Tools[toolName]; !exists {
			localReg.Tools[toolName] = registry.Tool{
				Description: fmt.Sprintf("Installed from %s", repo),
				Repo:        repo,
			}
			if saveErr := registry.SaveLocal(scutaDir, localReg); saveErr != nil {
				output.Debugf("Failed to save local registry: %v", saveErr)
			}
		}
	}

	// Record history
	start := time.Now()
	entry := history.NewEntry("install", true, time.Since(start), []history.ToolResult{
		{
			Name:    toolName,
			Action:  "install",
			Version: version,
			Success: true,
		},
	})
	if err := history.Record(scutaDir, entry); err != nil {
		output.Debugf("Failed to record history: %v", err)
	}

	_ = cmd // kept for signature consistency

	output.Success("Installed %s %s (from %s)", toolName, version, repo)
	return nil
}
