package managers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/sid-technologies/scuta/lib/audit"
)

// brewPrefixes are the standard Homebrew install locations, checked when
// HOMEBREW_PREFIX is not set.
var brewPrefixes = []string{"/opt/homebrew", "/usr/local", "/home/linuxbrew/.linuxbrew"}

// Brew audits formulae installed by Homebrew. Everything is read straight
// from the Cellar's INSTALL_RECEIPT.json files — no `brew` commands, which
// are far too slow to run per formula.
//
// Homebrew keeps no per-file manifest of installed content, so integrity
// is reported as not verifiable: tampering with a poured keg cannot be
// detected from local data.
type Brew struct {
	getenv   func(string) string
	readDir  func(string) ([]os.DirEntry, error)
	readFile func(string) ([]byte, error)
}

// NewBrew returns a Brew backed by the real OS.
func NewBrew() *Brew {
	return &Brew{
		getenv:   os.Getenv,
		readDir:  os.ReadDir,
		readFile: os.ReadFile,
	}
}

// Name implements Manager.
func (*Brew) Name() string { return "brew" }

// cellar returns the Cellar directory, or "" when Homebrew is not present.
func (b *Brew) cellar() string {
	if p := b.getenv("HOMEBREW_PREFIX"); p != "" {
		return filepath.Join(p, "Cellar")
	}
	for _, p := range brewPrefixes {
		c := filepath.Join(p, "Cellar")
		if entries, err := b.readDir(c); err == nil && len(entries) > 0 {
			return c
		}
	}
	return ""
}

// Detect implements Manager.
func (b *Brew) Detect() bool { return b.cellar() != "" }

// installReceipt is the subset of INSTALL_RECEIPT.json this adapter reads.
type installReceipt struct {
	PouredFromBottle bool `json:"poured_from_bottle"`
	Source           struct {
		Tap string `json:"tap"`
	} `json:"source"`
}

// Audit implements Manager.
func (b *Brew) Audit(_ context.Context) audit.ManagerReport {
	rep := audit.ManagerReport{Name: b.Name(), Detected: true}

	cellar := b.cellar()
	formulae, err := b.readDir(cellar)
	if err != nil {
		rep.Findings = append(rep.Findings, audit.Finding{
			Severity: audit.SeverityWarning,
			Code:     audit.CodeManagerUnreadable,
			Message:  fmt.Sprintf("could not read Cellar %s: %v", cellar, err),
		})
		return rep
	}

	for _, formula := range formulae {
		if !formula.IsDir() {
			continue
		}
		rep.Packages = append(rep.Packages, b.auditFormula(cellar, formula.Name())...)
	}

	sort.Slice(rep.Packages, func(i, j int) bool { return rep.Packages[i].Name < rep.Packages[j].Name })

	return rep
}

// auditFormula reads every installed version keg of one formula.
func (b *Brew) auditFormula(cellar, name string) []audit.SystemPackage {
	formulaDir := filepath.Join(cellar, name)
	versions, err := b.readDir(formulaDir)
	if err != nil {
		return nil
	}

	var out []audit.SystemPackage
	for _, v := range versions {
		if !v.IsDir() {
			continue
		}

		pkg := audit.SystemPackage{
			Name:       name,
			Version:    v.Name(),
			BinaryPath: filepath.Join(formulaDir, v.Name()),
			// Homebrew has no per-file manifest; keg tampering is not
			// detectable from local data.
			Integrity: audit.IntegrityNotVerifiable,
		}

		receipt, err := b.readReceipt(filepath.Join(formulaDir, v.Name(), "INSTALL_RECEIPT.json"))
		if err != nil {
			pkg.Integrity = audit.IntegrityUnknown
			pkg.Findings = append(pkg.Findings, audit.Finding{
				Severity: audit.SeverityWarning,
				Code:     audit.CodeNoIntegrityData,
				Message:  fmt.Sprintf("install receipt missing or unreadable: %v", err),
			})
			out = append(out, pkg)
			continue
		}

		pkg.Source = receipt.Source.Tap

		if tap := receipt.Source.Tap; tap != "" && tap != "homebrew/core" && tap != "homebrew/cask" {
			pkg.Findings = append(pkg.Findings, audit.Finding{
				Severity: audit.SeverityInfo,
				Code:     audit.CodeThirdPartySource,
				Message:  fmt.Sprintf("installed from third-party tap %s", tap),
			})
		}
		if !receipt.PouredFromBottle {
			pkg.Findings = append(pkg.Findings, audit.Finding{
				Severity: audit.SeverityInfo,
				Code:     audit.CodeUnpinnedBuild,
				Message:  "built from source locally, not poured from a bottle",
			})
		}

		out = append(out, pkg)
	}

	return out
}

// readReceipt parses one INSTALL_RECEIPT.json.
func (b *Brew) readReceipt(path string) (*installReceipt, error) {
	data, err := b.readFile(path)
	if err != nil {
		return nil, err
	}
	var r installReceipt
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("parse %s: %w", filepath.Base(path), err)
	}
	return &r, nil
}
