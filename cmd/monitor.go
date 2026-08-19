package cmd

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/sid-technologies/scuta/lib/monitor"
	"github.com/sid-technologies/scuta/lib/output"
	"github.com/sid-technologies/scuta/lib/path"

	"github.com/spf13/cobra"
)

// MonitorCmd schedules recurring audits with the OS scheduler: a launchd
// agent on macOS, a systemd user timer on Linux. Sender-only by design —
// scuta writes a local report file and your own collector ships it.
func MonitorCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "monitor",
		Short: "Schedule recurring security audits (no daemon)",
		Long: `Schedule 'scuta doctor --audit' with the operating system's own
scheduler: a launchd agent on macOS, a systemd user timer on Linux.
No daemon and no resident process — the scheduler runs the audit and
scuta exits.

Each run writes the JSON report atomically to a local file (default
~/.scuta/last-audit.json). Point your log shipper or a cron job at that
file to aggregate a fleet; see docs/FLEET.md. Scuta never uploads
anything itself.`,
	}

	cmd.AddCommand(monitorInstallCmd())
	cmd.AddCommand(monitorUninstallCmd())
	cmd.AddCommand(monitorStatusCmd())

	return cmd
}

//nolint:gochecknoinits // Standard Cobra pattern
func init() {
	rootCmd.AddCommand(MonitorCmd())
}

func monitorInstallCmd() *cobra.Command {
	var interval time.Duration
	var outPath, signKey string
	var system, skipCVE bool

	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install the recurring audit schedule",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runMonitorInstall(interval, outPath, signKey, system, skipCVE)
		},
	}

	cmd.Flags().DurationVar(&interval, "interval", time.Hour, "Time between audit runs (minimum 1m)")
	cmd.Flags().StringVar(&outPath, "output", "", "Report file the scheduled audit writes (default ~/.scuta/last-audit.json)")
	cmd.Flags().StringVar(&signKey, "sign-key", "", "Sign each report with this Ed25519 private key (see 'scuta admin keygen')")
	cmd.Flags().BoolVar(&system, "system", false, "Include the system package audit in scheduled runs")
	cmd.Flags().BoolVar(&skipCVE, "skip-cve", false, "Skip CVE lookups in scheduled runs (offline machines)")

	return cmd
}

func runMonitorInstall(interval time.Duration, outPath, signKey string, system, skipCVE bool) error {
	env, err := monitor.SystemEnv()
	if err != nil {
		return err
	}

	if outPath == "" {
		scutaDir, err := path.ScutaDir()
		if err != nil {
			return err
		}
		outPath = filepath.Join(scutaDir, "last-audit.json")
	}
	outPath, err = filepath.Abs(outPath)
	if err != nil {
		return err
	}
	if signKey != "" {
		if signKey, err = filepath.Abs(signKey); err != nil {
			return err
		}
	}

	args := []string{"doctor", "--audit", "--if-changed", "--output", outPath}
	if system {
		args = append(args, "--system")
	}
	if skipCVE {
		args = append(args, "--skip-cve")
	}
	if signKey != "" {
		args = append(args, "--sign-key", signKey)
	}

	paths, err := monitor.Install(env, monitor.Options{Interval: interval, AuditArgs: args})
	if err != nil {
		return err
	}

	output.Success("Scheduled audit every %s", interval)
	for _, p := range paths {
		output.Info("  %s", p)
	}
	output.Info("Report file: %s (ship it with your own collector — see docs/FLEET.md)", outPath)
	output.Dimmed("A run with critical findings exits non-zero, which the scheduler surfaces as a failed job.")

	return nil
}

func monitorUninstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall",
		Short: "Remove the recurring audit schedule",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			env, err := monitor.SystemEnv()
			if err != nil {
				return err
			}
			if err := monitor.Uninstall(env); err != nil {
				return err
			}
			output.Success("Removed the scheduled audit")
			return nil
		},
	}
}

func monitorStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show whether a recurring audit is scheduled",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			env, err := monitor.SystemEnv()
			if err != nil {
				return err
			}

			installed, paths := monitor.Installed(env)
			if len(paths) == 0 {
				return fmt.Errorf("scuta monitor does not support %s yet", env.GOOS)
			}
			if installed {
				output.Success("Scheduled audit installed")
			} else {
				output.Info("No scheduled audit installed")
			}
			for _, p := range paths {
				output.Dimmed("  %s", p)
			}
			return nil
		},
	}
}
