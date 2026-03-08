package cmd

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

func VersionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(_ *cobra.Command, _ []string) {
			fmt.Printf("scuta %s (%s/%s)\n", version, runtime.GOOS, runtime.GOARCH)
		},
	}

	return cmd
}

//nolint:gochecknoinits // Standard Cobra pattern
func init() {
	rootCmd.AddCommand(VersionCmd())
}
