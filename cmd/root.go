package cmd

import (
	"os"

	"github.com/sid-technologies/scuta/lib/exitcodes"
	"github.com/sid-technologies/scuta/lib/output"

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
