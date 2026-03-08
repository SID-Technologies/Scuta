package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/sid-technologies/scuta/lib/auth"
	"github.com/sid-technologies/scuta/lib/config"
	"github.com/sid-technologies/scuta/lib/exitcodes"
	"github.com/sid-technologies/scuta/lib/github"
	"github.com/sid-technologies/scuta/lib/output"
	"github.com/sid-technologies/scuta/lib/path"
	"github.com/sid-technologies/scuta/lib/state"
	"github.com/sid-technologies/scuta/lib/updater"

	"github.com/spf13/cobra"
)

// Output mode flags.
var (
	verboseFlag bool
	quietFlag   bool
	jsonFlag    bool
)

// version is set at build time via ldflags:
// go build -ldflags="-X github.com/sid-technologies/scuta/cmd.version=v1.0.0"
var version = "dev"

// rootCmd represents the base command when called without any subcommands.
var rootCmd = &cobra.Command{
	Use:   "scuta",
	Short: "SID Developer Toolbox",
	Long: `Scuta - Install once, get everything.

A unified CLI that manages SID's developer tools. Install, update,
and discover all SID CLI tools from a single command.

Getting started:

  scuta init                       Setup Scuta on your machine
  scuta install --all              Install all available tools
  scuta list                       See what's available

Management:

  scuta update                     Update all tools
  scuta doctor                     Health check
  scuta self-update                Update Scuta itself`,
	PersistentPreRun: func(_ *cobra.Command, _ []string) {
		checkForSelfUpdate()
	},
}

// Execute runs the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		code := exitcodes.CodeFrom(err)
		//nolint:revive // standard practice to use os.Exit in main package
		os.Exit(code)
	}
}

//nolint:gochecknoinits // Standard Cobra pattern for initializing commands
func init() {
	cobra.OnInitialize(initOutputMode)

	rootCmd.PersistentFlags().BoolVarP(&verboseFlag, "verbose", "v", false, "Show detailed output")
	rootCmd.PersistentFlags().BoolVarP(&quietFlag, "quiet", "q", false, "Suppress non-error output")
	rootCmd.PersistentFlags().BoolVar(&jsonFlag, "json", false, "Output in JSON format")

	rootCmd.MarkFlagsMutuallyExclusive("verbose", "quiet", "json")

	rootCmd.SetVersionTemplate("scuta {{.Version}}\n")
	rootCmd.Version = version
}

// initOutputMode sets the global output mode based on flags.
func initOutputMode() {
	switch {
	case jsonFlag:
		output.SetMode(output.ModeJSON)
	case quietFlag:
		output.SetMode(output.ModeQuiet)
	case verboseFlag:
		output.SetMode(output.ModeVerbose)
	default:
		output.SetMode(output.ModeNormal)
	}
}

// checkForSelfUpdate performs a passive update check on every command run.
// It prints to stderr and only checks once every update_interval (default 24h).
func checkForSelfUpdate() {
	// Skip in quiet/json mode, or for commands where it's not useful
	if output.IsQuiet() || output.IsJSON() {
		return
	}

	// Skip for self-update, version, and init commands
	if len(os.Args) > 1 {
		cmd := os.Args[1]
		if cmd == "self-update" || cmd == "version" || cmd == "init" {
			return
		}
	}

	// Don't check dev builds or non-semver versions (e.g., git hashes)
	if version == "dev" || !strings.Contains(version, ".") {
		return
	}

	scutaDir, err := path.ScutaDir()
	if err != nil {
		return
	}

	st, err := state.Load(scutaDir)
	if err != nil {
		return
	}

	cfg, err := config.Load(scutaDir)
	if err != nil {
		return
	}

	if !updater.NeedsCheck(st.LastUpdateCheck, cfg.UpdateIntervalDuration()) {
		return
	}

	// Perform the check
	token := auth.ResolveTokenWithConfig(scutaDir)
	ghClient := github.NewClient(token)
	upd := updater.New(ghClient)

	update, err := upd.CheckSelfUpdate(context.Background(), version)
	if err != nil {
		output.Debugf("Update check failed: %v", err)
		return
	}

	if update != nil {
		fmt.Fprintf(os.Stderr, "%s%s Update available: scuta %s → %s. Run: scuta self-update%s\n",
			output.WarningColor, output.SymbolWarning,
			update.CurrentVersion, update.LatestVersion,
			output.Reset,
		)
	}

	// Save updated check timestamp
	st.LastUpdateCheck = time.Now()
	_ = st.Save(scutaDir)
}
