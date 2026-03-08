package cmd

import (
	"fmt"
	"os"

	"github.com/sid-technologies/scuta/lib/auth"
	"github.com/sid-technologies/scuta/lib/output"
	"github.com/sid-technologies/scuta/lib/path"
	"github.com/sid-technologies/scuta/lib/registry"
	"github.com/sid-technologies/scuta/lib/state"

	"github.com/spf13/cobra"
)

func DoctorCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Health check — diagnose common issues",
		Long: `Checks:
  - ~/.scuta/bin/ exists and is in PATH
  - All installed binaries are executable
  - State file is valid
  - GitHub authentication is configured
  - Registry is reachable`,
		RunE: runDoctor,
	}

	return cmd
}

//nolint:gochecknoinits // Standard Cobra pattern
func init() {
	rootCmd.AddCommand(DoctorCmd())
}

func runDoctor(_ *cobra.Command, _ []string) error {
	output.Header("Scuta Doctor")

	issues := 0

	// 1. Check ~/.scuta/bin/ exists
	binDir, err := path.BinDir()
	if err != nil {
		printCheck(false, "~/.scuta/bin/ directory")
		issues++
	} else {
		if _, err := os.Stat(binDir); os.IsNotExist(err) {
			printCheck(false, "~/.scuta/bin/ directory exists")
			output.Dimmed("  Run 'scuta init' to create it")
			issues++
		} else {
			printCheck(true, "~/.scuta/bin/ directory exists")
		}
	}

	// 2. Check ~/.scuta/bin/ is in PATH
	if binDir != "" && isInPath(binDir) {
		printCheck(true, "~/.scuta/bin/ is in PATH")
	} else {
		printCheckWarn("~/.scuta/bin/ is not in PATH")
		output.Dimmed("  Run 'scuta init' for setup instructions")
		issues++
	}

	// 3. Check state file
	scutaDir, err := path.ScutaDir()
	if err != nil {
		printCheck(false, "Scuta directory")
		return nil
	}

	st, err := state.Load(scutaDir)
	if err != nil {
		printCheck(false, "State file is valid")
		issues++
	} else {
		printCheck(true, "State file is valid")
	}

	// 4. Check installed binaries exist and are executable
	if st != nil && len(st.Tools) > 0 {
		allGood := true
		for name, ts := range st.Tools {
			if ts.BinaryPath == "" {
				continue
			}
			info, err := os.Stat(ts.BinaryPath)
			if err != nil {
				printCheck(false, "%s binary exists at %s", name, ts.BinaryPath)
				allGood = false
				issues++
				continue
			}
			if info.Mode()&0o111 == 0 {
				printCheck(false, "%s binary is executable", name)
				allGood = false
				issues++
				continue
			}
		}
		if allGood {
			printCheck(true, "All installed binaries exist and are executable")
		}
	} else {
		output.Dimmed("  No tools installed yet")
	}

	// 5. Check GitHub auth
	token := auth.ResolveTokenWithConfig(scutaDir)
	if token != "" {
		printCheck(true, "GitHub authentication configured")
	} else {
		printCheckWarn("GitHub authentication not configured")
		output.Dimmed("  Set SCUTA_GITHUB_TOKEN or install gh CLI for private repo access")
		issues++
	}

	// 6. Check registry
	_, err = registry.Load()
	if err != nil {
		printCheck(false, "Registry is loadable")
		issues++
	} else {
		printCheck(true, "Registry is loadable")
	}

	// Summary
	fmt.Println()
	if issues == 0 {
		output.Success("Everything looks good!")
	} else {
		output.Warning("%d issue(s) found", issues)
	}

	return nil
}

func printCheck(ok bool, msg string, args ...any) {
	formatted := fmt.Sprintf(msg, args...)
	if ok {
		fmt.Printf("  %s%s%s %s\n", output.SuccessColor, output.SymbolSuccess, output.Reset, formatted)
	} else {
		fmt.Printf("  %s%s%s %s\n", output.ErrorColor, output.SymbolFailure, output.Reset, formatted)
	}
}

func printCheckWarn(msg string, args ...any) {
	formatted := fmt.Sprintf(msg, args...)
	fmt.Printf("  %s%s%s %s\n", output.WarningColor, output.SymbolWarning, output.Reset, formatted)
}
