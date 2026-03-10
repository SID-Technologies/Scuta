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
  - Registry is reachable
  - Policy compliance (version constraints)`,
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
		output.PrintCheck(false, "~/.scuta/bin/ directory")
		issues++
	} else {
		if _, err := os.Stat(binDir); os.IsNotExist(err) {
			output.PrintCheck(false, "~/.scuta/bin/ directory exists")
			output.Dimmed("  Run 'scuta init' to create it")
			issues++
		} else {
			output.PrintCheck(true, "~/.scuta/bin/ directory exists")
		}
	}

	// 2. Check ~/.scuta/bin/ is in PATH
	if binDir != "" && isInPath(binDir) {
		output.PrintCheck(true, "~/.scuta/bin/ is in PATH")
	} else {
		output.PrintCheckWarn("~/.scuta/bin/ is not in PATH")
		output.Dimmed("  Run 'scuta init' for setup instructions")
		issues++
	}

	// 3. Check state file
	scutaDir, err := path.ScutaDir()
	if err != nil {
		output.PrintCheck(false, "Scuta directory")
		return nil
	}

	st, err := state.Load(scutaDir)
	if err != nil {
		output.PrintCheck(false, "State file is valid")
		issues++
	} else {
		output.PrintCheck(true, "State file is valid")
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
				output.PrintCheck(false, "%s binary exists at %s", name, ts.BinaryPath)
				allGood = false
				issues++
				continue
			}
			if info.Mode()&0o111 == 0 {
				output.PrintCheck(false, "%s binary is executable", name)
				allGood = false
				issues++
				continue
			}
		}
		if allGood {
			output.PrintCheck(true, "All installed binaries exist and are executable")
		}
	} else {
		output.Dimmed("  No tools installed yet")
	}

	// 5. Check GitHub auth
	token := auth.ResolveTokenWithConfig(scutaDir)
	if token != "" {
		output.PrintCheck(true, "GitHub authentication configured")
	} else {
		output.PrintCheckWarn("GitHub authentication not configured")
		output.Dimmed("  Set SCUTA_GITHUB_TOKEN or install gh CLI for private repo access")
		issues++
	}

	// 6. Check registry
	_, err = registry.Load()
	if err != nil {
		output.PrintCheck(false, "Registry is loadable")
		issues++
	} else {
		output.PrintCheck(true, "Registry is loadable")
	}

	// 7. Policy compliance
	pol := loadPolicy(scutaDir)
	if pol != nil {
		// Check Scuta version
		if v := pol.CheckScutaVersion(version); v != nil {
			output.PrintCheck(false, "Scuta version meets policy minimum (%s)", v.Message)
			issues++
		} else if pol.MinScutaVersion != "" {
			output.PrintCheck(true, "Scuta version meets policy minimum (>= %s)", pol.MinScutaVersion)
		}

		// Check all installed tool versions
		if st != nil && len(st.Tools) > 0 {
			installed := make(map[string]string, len(st.Tools))
			for name, ts := range st.Tools {
				installed[name] = ts.Version
			}

			violations := pol.CheckAll(installed)
			if len(violations) == 0 {
				output.PrintCheck(true, "All installed tools comply with policy")
			} else {
				for _, v := range violations {
					output.PrintCheck(false, "%s %s: %s", v.Tool, v.Version, v.Message)
					issues++
				}
			}
		}
	} else {
		output.Dimmed("  No policy configured")
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
