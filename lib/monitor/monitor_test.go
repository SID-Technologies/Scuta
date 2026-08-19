package monitor

import (
	"os"
	"strings"
	"testing"
	"time"
)

// fakeFS records writes/removes/commands for a fake Env.
type fakeFS struct {
	files    map[string]string
	removed  []string
	commands [][]string
	runErr   error
}

func newFakeEnv(goos string, fs *fakeFS) Env {
	if fs.files == nil {
		fs.files = map[string]string{}
	}
	return Env{
		GOOS:       goos,
		HomeDir:    "/home/u",
		UID:        "501",
		Executable: "/home/u/.scuta/bin/scuta",
		WriteFile: func(name string, data []byte, _ os.FileMode) error {
			fs.files[name] = string(data)
			return nil
		},
		MkdirAll: func(string, os.FileMode) error { return nil },
		Remove: func(name string) error {
			if _, ok := fs.files[name]; !ok {
				return os.ErrNotExist
			}
			delete(fs.files, name)
			fs.removed = append(fs.removed, name)
			return nil
		},
		Stat: func(name string) (os.FileInfo, error) {
			if _, ok := fs.files[name]; ok {
				return nil, nil //nolint:nilnil // only existence matters here
			}
			return nil, os.ErrNotExist
		},
		Run: func(name string, args ...string) error {
			fs.commands = append(fs.commands, append([]string{name}, args...))
			return fs.runErr
		},
	}
}

func defaultOpts() Options {
	return Options{
		Interval:  30 * time.Minute,
		AuditArgs: []string{"doctor", "--audit", "--output", "/home/u/.scuta/last audit.json"},
	}
}

func TestInstallRejectsSubMinuteInterval(t *testing.T) {
	env := newFakeEnv("darwin", &fakeFS{})
	if _, err := Install(env, Options{Interval: 30 * time.Second}); err == nil {
		t.Fatal("expected error for sub-minute interval")
	}
}

func TestInstallUnsupportedPlatform(t *testing.T) {
	env := newFakeEnv("windows", &fakeFS{})
	if _, err := Install(env, defaultOpts()); err == nil || !strings.Contains(err.Error(), "windows") {
		t.Fatalf("err = %v", err)
	}
}

func TestInstallLaunchd(t *testing.T) {
	fs := &fakeFS{}
	env := newFakeEnv("darwin", fs)

	paths, err := Install(env, defaultOpts())
	if err != nil {
		t.Fatalf("Install: %v", err)
	}

	want := "/home/u/Library/LaunchAgents/com.sid-technologies.scuta.audit.plist"
	if len(paths) != 1 || paths[0] != want {
		t.Fatalf("paths = %v", paths)
	}

	plist := fs.files[want]
	for _, frag := range []string{
		"<string>com.sid-technologies.scuta.audit</string>",
		"<string>/home/u/.scuta/bin/scuta</string>",
		"<string>doctor</string>",
		"<string>--audit</string>",
		"<string>/home/u/.scuta/last audit.json</string>",
		"<integer>1800</integer>",
		"<string>/home/u/.scuta/monitor.log</string>",
	} {
		if !strings.Contains(plist, frag) {
			t.Fatalf("plist missing %q:\n%s", frag, plist)
		}
	}

	// Reload sequence: bootout (tolerated), then bootstrap.
	if len(fs.commands) != 2 || fs.commands[1][1] != "bootstrap" {
		t.Fatalf("commands = %v", fs.commands)
	}
}

func TestInstallLaunchdEscapesXML(t *testing.T) {
	fs := &fakeFS{}
	env := newFakeEnv("darwin", fs)

	opts := defaultOpts()
	opts.AuditArgs = []string{"doctor", "--audit", "--output", "/tmp/a&b.json"}
	if _, err := Install(env, opts); err != nil {
		t.Fatalf("Install: %v", err)
	}

	plist := fs.files["/home/u/Library/LaunchAgents/com.sid-technologies.scuta.audit.plist"]
	if !strings.Contains(plist, "/tmp/a&amp;b.json") {
		t.Fatalf("ampersand not escaped:\n%s", plist)
	}
}

func TestInstallLaunchdBootstrapFailureKeepsPlist(t *testing.T) {
	fs := &fakeFS{runErr: os.ErrPermission}
	env := newFakeEnv("darwin", fs)

	paths, err := Install(env, defaultOpts())
	if err == nil || !strings.Contains(err.Error(), "launchctl bootstrap gui/501") {
		t.Fatalf("err = %v", err)
	}
	if len(paths) != 1 {
		t.Fatalf("plist path should still be returned: %v", paths)
	}
	if _, ok := fs.files[paths[0]]; !ok {
		t.Fatal("plist should remain on disk for manual loading")
	}
}

func TestInstallSystemd(t *testing.T) {
	fs := &fakeFS{}
	env := newFakeEnv("linux", fs)

	paths, err := Install(env, defaultOpts())
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if len(paths) != 2 {
		t.Fatalf("paths = %v", paths)
	}

	service := fs.files["/home/u/.config/systemd/user/scuta-audit.service"]
	if !strings.Contains(service, "Type=oneshot") {
		t.Fatalf("service:\n%s", service)
	}
	if !strings.Contains(service, `ExecStart=/home/u/.scuta/bin/scuta doctor --audit --output "/home/u/.scuta/last audit.json"`) {
		t.Fatalf("ExecStart quoting wrong:\n%s", service)
	}

	timer := fs.files["/home/u/.config/systemd/user/scuta-audit.timer"]
	for _, frag := range []string{"OnUnitActiveSec=30min", "Persistent=true", "WantedBy=timers.target"} {
		if !strings.Contains(timer, frag) {
			t.Fatalf("timer missing %q:\n%s", frag, timer)
		}
	}

	// daemon-reload then enable --now.
	if len(fs.commands) != 2 || fs.commands[1][3] != "--now" {
		t.Fatalf("commands = %v", fs.commands)
	}
}

func TestUninstallDarwinIdempotent(t *testing.T) {
	fs := &fakeFS{}
	env := newFakeEnv("darwin", fs)

	if _, err := Install(env, defaultOpts()); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if err := Uninstall(env); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	if len(fs.files) != 0 {
		t.Fatalf("files remain: %v", fs.files)
	}
	// Second uninstall: nothing left to remove, still no error.
	if err := Uninstall(env); err != nil {
		t.Fatalf("second Uninstall: %v", err)
	}
}

func TestUninstallLinuxRemovesBothUnits(t *testing.T) {
	fs := &fakeFS{}
	env := newFakeEnv("linux", fs)

	if _, err := Install(env, defaultOpts()); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if err := Uninstall(env); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	if len(fs.files) != 0 {
		t.Fatalf("files remain: %v", fs.files)
	}
}

func TestInstalledReportsState(t *testing.T) {
	fs := &fakeFS{}
	env := newFakeEnv("linux", fs)

	ok, paths := Installed(env)
	if ok {
		t.Fatal("nothing installed yet")
	}
	if len(paths) != 2 {
		t.Fatalf("paths = %v", paths)
	}

	if _, err := Install(env, defaultOpts()); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if ok, _ := Installed(env); !ok {
		t.Fatal("should report installed")
	}
}
