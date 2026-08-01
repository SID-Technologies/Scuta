package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"

	"github.com/sid-technologies/scuta/lib/audit"
	"github.com/sid-technologies/scuta/lib/auth"
	"github.com/sid-technologies/scuta/lib/config"
	"github.com/sid-technologies/scuta/lib/cve"
	"github.com/sid-technologies/scuta/lib/exitcodes"
	"github.com/sid-technologies/scuta/lib/output"
	"github.com/sid-technologies/scuta/lib/path"
	"github.com/sid-technologies/scuta/lib/registry"
	"github.com/sid-technologies/scuta/lib/shellutil"
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
  - Policy compliance (version constraints)
  - Known vulnerabilities (via OSV.dev)

With --audit, runs a security audit instead: per-tool provenance
(was the install checksum-verified?), tamper detection (does the binary
on disk still match the hash recorded at install?), policy compliance,
CVEs, and machine posture (trust root, signed metadata, policy).
Combine with the global --json flag to emit the audit report as JSON
for fleet aggregation.

Audit exit codes: 0 clean or warnings only, 1 critical findings.`,
		RunE: runDoctor,
	}

	cmd.Flags().Bool("skip-cve", false, "Skip CVE vulnerability check (for offline environments)")
	cmd.Flags().Bool("audit", false, "Security audit: provenance, tamper detection, policy and posture")

	return cmd
}

//nolint:gochecknoinits // Standard Cobra pattern
func init() {
	rootCmd.AddCommand(DoctorCmd())
}

func runDoctor(cmd *cobra.Command, _ []string) error {
	if auditFlag, _ := cmd.Flags().GetBool("audit"); auditFlag {
		// The persistent --json flag switches the audit to machine output.
		return runDoctorAudit(cmd, jsonFlag)
	}

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
	if binDir != "" && shellutil.IsInPath(binDir) {
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
			// Windows has no POSIX execute bits; presence is sufficient there.
			if runtime.GOOS != "windows" && info.Mode()&0o111 == 0 {
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

	// 8. CVE vulnerability check
	skipCVE, _ := cmd.Flags().GetBool("skip-cve")
	if skipCVE {
		output.Dimmed("  CVE check skipped (--skip-cve)")
	} else if st != nil && len(st.Tools) > 0 {
		cveIssues := 0
		for name, ts := range st.Tools {
			vulns, err := cve.CheckWithCache(scutaDir, name, ts.Version, "Go")
			if err != nil {
				output.Debugf("CVE check failed for %s: %v", name, err)
				continue
			}
			if len(vulns) > 0 {
				for _, v := range vulns {
					output.PrintCheck(false, "%s %s: %s (%s)", name, ts.Version, v.ID, v.Summary)
					cveIssues++
					issues++
				}
			}
		}
		if cveIssues == 0 {
			output.PrintCheck(true, "No known vulnerabilities in installed tools")
		}
	}

	// 9. Telemetry status
	cfg, cfgErr := config.LoadWithMerge(scutaDir)
	if cfgErr == nil {
		if cfg.Telemetry {
			output.PrintCheck(true, "Telemetry is enabled (opt-in)")
		} else {
			output.Dimmed("  Telemetry is disabled (opt-in via: scuta config set telemetry true)")
		}
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

// runDoctorAudit builds and renders the security audit report.
// Critical findings exit non-zero so CI can gate on them.
func runDoctorAudit(cmd *cobra.Command, jsonOut bool) error {
	scutaDir, err := path.ScutaDir()
	if err != nil {
		return err
	}

	st, err := state.Load(scutaDir)
	if err != nil {
		return err
	}

	pol := loadPolicy(scutaDir)
	trusted := config.LoadTrusted(scutaDir)

	report := audit.New(version)
	report.Posture = audit.CheckPosture(audit.PostureInput{
		TrustRootConfigured:   trusted.SignaturePublicKey != "",
		RequireSignature:      trusted.RequireSignature,
		RequireSignedMetadata: trusted.RequireSignedMetadata,
		PolicyConfigured:      pol != nil,
		ConfigURL:             trusted.ConfigURL,
	})
	report.Tools = audit.CheckTools(st, pol)

	appendCVEFindings(cmd, scutaDir, report)
	report.Finalize()

	if jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			return err
		}
	} else {
		printAuditReport(report)
	}

	if report.Summary.Criticals > 0 {
		// Critical findings are a CI gate signal, not a usage error.
		cmd.SilenceUsage = true
		return exitcodes.NewError(exitcodes.General, fmt.Sprintf("audit found %d critical finding(s)", report.Summary.Criticals))
	}

	return nil
}

// appendCVEFindings attaches known-vulnerability findings per tool unless
// --skip-cve is set. CVE lookups are best-effort: failures are debug-logged,
// never fatal, so audits work offline.
func appendCVEFindings(cmd *cobra.Command, scutaDir string, report *audit.Report) {
	if skip, _ := cmd.Flags().GetBool("skip-cve"); skip {
		return
	}

	for i := range report.Tools {
		t := &report.Tools[i]
		vulns, err := cve.CheckWithCache(scutaDir, t.Name, t.Version, "Go")
		if err != nil {
			output.Debugf("CVE check failed for %s: %v", t.Name, err)
			continue
		}
		for _, v := range vulns {
			t.Findings = append(t.Findings, audit.Finding{
				Severity: audit.SeverityWarning,
				Code:     audit.CodeKnownVulnerability,
				Message:  fmt.Sprintf("%s: %s", v.ID, v.Summary),
			})
		}
	}
}

// printAuditReport renders the human-readable audit output.
func printAuditReport(report *audit.Report) {
	output.Header("Scuta Audit")

	output.Info("Machine posture:")
	output.PrintCheck(report.Posture.TrustRootConfigured, "Trust root configured (signature_public_key)")
	output.PrintCheck(report.Posture.RequireSignedMetadata, "Signed metadata required (require_signed_metadata)")
	output.PrintCheck(report.Posture.PolicyConfigured, "Policy configured")
	if report.Posture.ConfigURL != "" {
		output.Dimmed("  Org config: " + report.Posture.ConfigURL)
	}

	if len(report.Tools) == 0 {
		output.Dimmed("  No tools installed")
	}

	for i := range report.Tools {
		t := &report.Tools[i]
		fmt.Println()
		output.Info("%s %s", t.Name, t.Version)
		if len(t.Findings) == 0 {
			output.PrintCheck(true, "verified install, binary matches install-time hash")
			continue
		}
		for _, f := range t.Findings {
			switch f.Severity {
			case audit.SeverityCritical:
				output.PrintCheck(false, "%s", f.Message)
			case audit.SeverityWarning:
				output.PrintCheckWarn("%s", f.Message)
			default:
				output.Dimmed("  " + f.Message)
			}
		}
	}

	fmt.Println()
	switch {
	case report.Summary.Criticals > 0:
		output.Error("%d critical, %d warning finding(s)", report.Summary.Criticals, report.Summary.Warnings)
	case report.Summary.Warnings > 0:
		output.Warning("%d warning finding(s)", report.Summary.Warnings)
	default:
		output.Success("Audit clean — %d tool(s) checked", report.Summary.Tools)
	}
}
