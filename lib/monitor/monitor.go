// Package monitor turns the audit into a recurring posture feed using the
// operating system's own scheduler: a launchd agent on macOS, a systemd user
// timer on Linux. No daemon, no resident process — the scheduler invokes
// `scuta doctor --audit` on an interval and scuta exits.
package monitor

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Unit and label names for the generated scheduler entries.
const (
	launchdLabel = "com.sid-technologies.scuta.audit"
	systemdName  = "scuta-audit"
)

// Env abstracts the OS touchpoints so both platforms are testable from any
// host. Run activates scheduler changes; failures there are reported with
// manual instructions rather than treated as fatal, since the unit files are
// already in place.
type Env struct {
	GOOS       string
	HomeDir    string
	UID        string
	Executable string // absolute path to the scuta binary the timer will run
	WriteFile  func(name string, data []byte, perm os.FileMode) error
	MkdirAll   func(name string, perm os.FileMode) error
	Remove     func(name string) error
	Stat       func(name string) (os.FileInfo, error)
	Run        func(name string, args ...string) error
}

// SystemEnv returns an Env backed by the real OS.
func SystemEnv() (Env, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Env{}, err
	}
	exe, err := os.Executable()
	if err != nil {
		return Env{}, err
	}

	return Env{
		GOOS:       runtime.GOOS,
		HomeDir:    home,
		UID:        fmt.Sprintf("%d", os.Getuid()),
		Executable: exe,
		WriteFile:  os.WriteFile,
		MkdirAll:   os.MkdirAll,
		Remove:     os.Remove,
		Stat:       os.Stat,
		Run: func(name string, args ...string) error {
			return exec.Command(name, args...).Run()
		},
	}, nil
}

// Options configures the generated schedule.
type Options struct {
	Interval  time.Duration
	AuditArgs []string // arguments after the binary path, e.g. doctor --audit --output ...
}

// Install writes the scheduler entry for the current platform and activates
// it. It returns the paths written.
func Install(env Env, opts Options) ([]string, error) {
	if opts.Interval < time.Minute {
		return nil, fmt.Errorf("interval %s is below the 1m minimum", opts.Interval)
	}

	switch env.GOOS {
	case "darwin":
		return installLaunchd(env, opts)
	case "linux":
		return installSystemd(env, opts)
	default:
		return nil, fmt.Errorf("scuta monitor does not support %s yet; schedule `scuta doctor --audit` with the native task scheduler", env.GOOS)
	}
}

// Uninstall deactivates and removes the scheduler entry. Missing files are
// not an error: uninstall is idempotent.
func Uninstall(env Env) error {
	switch env.GOOS {
	case "darwin":
		// Deactivation failure is non-fatal: the agent may not be loaded.
		_ = env.Run("launchctl", "bootout", "gui/"+env.UID+"/"+launchdLabel)
		return removeAll(env, launchdPlistPath(env))
	case "linux":
		_ = env.Run("systemctl", "--user", "disable", "--now", systemdName+".timer")
		if err := removeAll(env, systemdTimerPath(env), systemdServicePath(env)); err != nil {
			return err
		}
		_ = env.Run("systemctl", "--user", "daemon-reload")
		return nil
	default:
		return fmt.Errorf("scuta monitor does not support %s yet", env.GOOS)
	}
}

// Installed reports whether the scheduler entry files are present, and their
// paths for display.
func Installed(env Env) (bool, []string) {
	var paths []string
	switch env.GOOS {
	case "darwin":
		paths = []string{launchdPlistPath(env)}
	case "linux":
		paths = []string{systemdTimerPath(env), systemdServicePath(env)}
	default:
		return false, nil
	}

	for _, p := range paths {
		if _, err := env.Stat(p); err != nil {
			return false, paths
		}
	}
	return true, paths
}

func launchdPlistPath(env Env) string {
	return filepath.Join(env.HomeDir, "Library", "LaunchAgents", launchdLabel+".plist")
}

func systemdUnitDir(env Env) string {
	return filepath.Join(env.HomeDir, ".config", "systemd", "user")
}

func systemdTimerPath(env Env) string {
	return filepath.Join(systemdUnitDir(env), systemdName+".timer")
}

func systemdServicePath(env Env) string {
	return filepath.Join(systemdUnitDir(env), systemdName+".service")
}

// installLaunchd writes a LaunchAgent plist and (re)loads it.
func installLaunchd(env Env, opts Options) ([]string, error) {
	plist := launchdPlist(env, opts)
	path := launchdPlistPath(env)

	if err := env.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	if err := env.WriteFile(path, []byte(plist), 0o644); err != nil {
		return nil, err
	}

	// Reload: bootout is expected to fail when the agent was not loaded.
	_ = env.Run("launchctl", "bootout", "gui/"+env.UID+"/"+launchdLabel)
	if err := env.Run("launchctl", "bootstrap", "gui/"+env.UID, path); err != nil {
		return []string{path}, fmt.Errorf("plist written, but launchctl bootstrap failed (%w); load it manually: launchctl bootstrap gui/%s %s", err, env.UID, path)
	}

	return []string{path}, nil
}

// launchdPlist renders the LaunchAgent definition. Arguments are XML-escaped;
// audit output goes to ~/.scuta/monitor.log.
func launchdPlist(env Env, opts Options) string {
	logPath := filepath.Join(env.HomeDir, ".scuta", "monitor.log")

	var args strings.Builder
	for _, a := range append([]string{env.Executable}, opts.AuditArgs...) {
		args.WriteString("    <string>" + xmlEscape(a) + "</string>\n")
	}

	return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>` + launchdLabel + `</string>
  <key>ProgramArguments</key>
  <array>
` + args.String() + `  </array>
  <key>StartInterval</key>
  <integer>` + fmt.Sprintf("%d", int(opts.Interval.Seconds())) + `</integer>
  <key>RunAtLoad</key>
  <true/>
  <key>StandardOutPath</key>
  <string>` + xmlEscape(logPath) + `</string>
  <key>StandardErrorPath</key>
  <string>` + xmlEscape(logPath) + `</string>
</dict>
</plist>
`
}

// installSystemd writes a user service + timer pair and enables the timer.
func installSystemd(env Env, opts Options) ([]string, error) {
	dir := systemdUnitDir(env)
	if err := env.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}

	service := "[Unit]\nDescription=Scuta security audit\n\n[Service]\nType=oneshot\nExecStart=" +
		systemdExecStart(env, opts) + "\n"
	timer := "[Unit]\nDescription=Run the scuta security audit on an interval\n\n[Timer]\nOnBootSec=2min\nOnUnitActiveSec=" +
		fmt.Sprintf("%dmin", int(opts.Interval.Minutes())) + "\nPersistent=true\n\n[Install]\nWantedBy=timers.target\n"

	paths := []string{systemdServicePath(env), systemdTimerPath(env)}
	if err := env.WriteFile(paths[0], []byte(service), 0o644); err != nil {
		return nil, err
	}
	if err := env.WriteFile(paths[1], []byte(timer), 0o644); err != nil {
		return nil, err
	}

	if err := env.Run("systemctl", "--user", "daemon-reload"); err != nil {
		return paths, fmt.Errorf("units written, but daemon-reload failed (%w); enable manually: systemctl --user enable --now %s.timer", err, systemdName)
	}
	if err := env.Run("systemctl", "--user", "enable", "--now", systemdName+".timer"); err != nil {
		return paths, fmt.Errorf("units written, but enabling failed (%w); enable manually: systemctl --user enable --now %s.timer", err, systemdName)
	}

	return paths, nil
}

// systemdExecStart quotes the command line per systemd unit syntax: space
// separated, double quotes around arguments containing whitespace.
func systemdExecStart(env Env, opts Options) string {
	parts := make([]string, 0, len(opts.AuditArgs)+1)
	for _, a := range append([]string{env.Executable}, opts.AuditArgs...) {
		if strings.ContainsAny(a, " \t") {
			a = `"` + a + `"`
		}
		parts = append(parts, a)
	}
	return strings.Join(parts, " ")
}

func removeAll(env Env, paths ...string) error {
	for _, p := range paths {
		if err := env.Remove(p); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func xmlEscape(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}
