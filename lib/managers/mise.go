package managers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/sid-technologies/scuta/lib/audit"
)

// Mise audits tools installed by mise (formerly rtx). Inventory comes from
// the data directory's installs tree. mise records checksums in per-project
// lockfiles, not in a machine-global store, so integrity is reported as not
// verifiable from a machine-level audit.
type Mise struct {
	getenv      func(string) string
	userHomeDir func() (string, error)
	readDir     func(string) ([]os.DirEntry, error)
}

// NewMise returns a Mise backed by the real OS.
func NewMise() *Mise {
	return &Mise{
		getenv:      os.Getenv,
		userHomeDir: os.UserHomeDir,
		readDir:     os.ReadDir,
	}
}

// Name implements Manager.
func (*Mise) Name() string { return "mise" }

// installsDir resolves the mise installs tree: MISE_DATA_DIR, then
// XDG_DATA_HOME, then ~/.local/share.
func (m *Mise) installsDir() string {
	if d := m.getenv("MISE_DATA_DIR"); d != "" {
		return filepath.Join(d, "installs")
	}
	if d := m.getenv("XDG_DATA_HOME"); d != "" {
		return filepath.Join(d, "mise", "installs")
	}
	home, err := m.userHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".local", "share", "mise", "installs")
}

// Detect implements Manager.
func (m *Mise) Detect() bool {
	dir := m.installsDir()
	if dir == "" {
		return false
	}
	entries, err := m.readDir(dir)
	return err == nil && len(entries) > 0
}

// Audit implements Manager.
func (m *Mise) Audit(_ context.Context) audit.ManagerReport {
	rep := audit.ManagerReport{Name: m.Name(), Detected: true}

	dir := m.installsDir()
	tools, err := m.readDir(dir)
	if err != nil {
		rep.Findings = append(rep.Findings, audit.Finding{
			Severity: audit.SeverityWarning,
			Code:     audit.CodeManagerUnreadable,
			Message:  fmt.Sprintf("could not read mise installs %s: %v", dir, err),
		})
		return rep
	}

	for _, tool := range tools {
		if !tool.IsDir() {
			continue
		}
		toolDir := filepath.Join(dir, tool.Name())
		versions, err := m.readDir(toolDir)
		if err != nil {
			continue
		}
		for _, v := range versions {
			// mise symlinks partial versions (18, 18.20) to the full
			// install (18.20.2); only real directories are installs.
			if !v.IsDir() || v.Type()&os.ModeSymlink != 0 {
				continue
			}
			rep.Packages = append(rep.Packages, audit.SystemPackage{
				Name:       tool.Name(),
				Version:    v.Name(),
				BinaryPath: filepath.Join(toolDir, v.Name()),
				// Checksums live in per-project mise.lock files, which a
				// machine-level audit cannot discover.
				Integrity: audit.IntegrityNotVerifiable,
			})
		}
	}

	sort.Slice(rep.Packages, func(i, j int) bool {
		if rep.Packages[i].Name != rep.Packages[j].Name {
			return rep.Packages[i].Name < rep.Packages[j].Name
		}
		return rep.Packages[i].Version < rep.Packages[j].Version
	})

	return rep
}
