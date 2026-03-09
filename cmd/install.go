package cmd

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/sid-technologies/scuta/lib/auth"
	"github.com/sid-technologies/scuta/lib/github"
	"github.com/sid-technologies/scuta/lib/graph"
	"github.com/sid-technologies/scuta/lib/history"
	"github.com/sid-technologies/scuta/lib/installer"
	"github.com/sid-technologies/scuta/lib/lock"
	"github.com/sid-technologies/scuta/lib/output"
	"github.com/sid-technologies/scuta/lib/path"
	"github.com/sid-technologies/scuta/lib/registry"
	"github.com/sid-technologies/scuta/lib/state"
	"github.com/sid-technologies/scuta/lib/suggest"
	workerqueue "github.com/sid-technologies/scuta/lib/worker_queue"

	"github.com/spf13/cobra"
)

func InstallCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "install <tool>",
		Short: "Install a tool from the registry",
		Long: `Downloads the correct binary for your OS/architecture from the tool's
GitHub Releases, verifies checksum, and places it in ~/.scuta/bin/.`,
		Args: cobra.MaximumNArgs(1),
		RunE: runInstall,
	}

	cmd.Flags().Bool("all", false, "Install all tools from registry")
	cmd.Flags().String("version", "", "Install a specific version (default: latest)")
	cmd.Flags().Bool("force", false, "Reinstall even if already installed")
	cmd.Flags().Bool("skip-verify", false, "Skip checksum verification")
	cmd.Flags().Bool("dry-run", false, "Show what would be installed without installing")
	cmd.Flags().String("from", "", "Install from a local archive file (offline/air-gapped)")

	return cmd
}

//nolint:gochecknoinits // Standard Cobra pattern
func init() {
	rootCmd.AddCommand(InstallCmd())
}

func runInstall(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithCancel(cmd.Context())
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		output.Warning("\nInterrupted, cleaning up...")
		cancel()
	}()
	defer signal.Stop(sigChan)
	defer cancel()

	allFlag, _ := cmd.Flags().GetBool("all")
	versionFlag, _ := cmd.Flags().GetString("version")
	forceFlag, _ := cmd.Flags().GetBool("force")
	skipVerifyFlag, _ := cmd.Flags().GetBool("skip-verify")
	dryRunFlag, _ := cmd.Flags().GetBool("dry-run")
	fromFlag, _ := cmd.Flags().GetString("from")

	// Offline install from local archive
	if fromFlag != "" {
		return runInstallFromArchive(ctx, args, fromFlag)
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

	// Sort by dependency order using the graph
	toolNames = orderByDependencies(toolNames, reg)

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
	ghClient := newGitHubClient(token, scutaDir)
	inst := installer.New(ghClient, scutaDir)

	start := time.Now()

	// Use parallel installs for --all with multiple tools, sequential otherwise
	var toolResults []history.ToolResult
	var successCount int

	if dryRunFlag {
		toolResults, successCount = installDryRun(ctx, toolNames, reg, ghClient, versionFlag)
	} else if allFlag && len(toolNames) > 1 {
		toolResults, successCount = installParallel(ctx, toolNames, reg, inst, st, versionFlag, forceFlag, skipVerifyFlag)
	} else {
		toolResults, successCount = installSequential(ctx, toolNames, reg, inst, st, versionFlag, forceFlag, skipVerifyFlag)
	}

	// Skip state save and history if canceled or dry-run
	if ctx.Err() != nil || dryRunFlag {
		return nil
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
		result := installOneTool(ctx, toolName, reg, inst, st, versionFlag, forceFlag, skipVerifyFlag)
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
			result := installOneTool(ctx, group[0], reg, inst, st, versionFlag, forceFlag, skipVerifyFlag)
			allResults = append(allResults, result)
			if result.Success {
				totalSuccess++
			}
			continue
		}

		// Multiple tools at this depth — run in parallel
		wq := workerqueue.NewWorkQueue(func(task *workerqueue.TaskInfo) bool {
			result := installOneTool(ctx, task.ToolName, reg, inst, st, versionFlag, forceFlag, skipVerifyFlag)
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

	output.Info("Installing %s...", toolName)

	result, err := inst.Install(ctx, toolName, tool.Repo, versionFlag, forceFlag, skipVerifyFlag)
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
		output.Error("specify a tool name when using --from (e.g., scuta install pilum --from ./archive.tar.gz)")
		return nil
	}

	toolName := args[0]

	// Validate archive file exists
	if _, err := os.Stat(archivePath); err != nil {
		output.Error("archive file not found: %s", archivePath)
		return nil
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
		output.Error("Failed to install %s: %v", toolName, err)
		return nil
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
