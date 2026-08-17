package managers

import (
	"context"
	"debug/buildinfo"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"

	"github.com/sid-technologies/scuta/lib/audit"
)

// develVersion is what the toolchain stamps on binaries built from a
// source checkout rather than an installed module version.
const develVersion = "(devel)"

// GoBin audits binaries installed by `go install` into GOBIN. Everything
// it reports comes from build metadata the toolchain embeds in the binary
// itself (debug/buildinfo), so no commands are executed.
type GoBin struct {
	getenv        func(string) string
	userHomeDir   func() (string, error)
	readDir       func(string) ([]os.DirEntry, error)
	readBuildInfo func(string) (*debug.BuildInfo, error)
}

// NewGoBin returns a GoBin backed by the real OS.
func NewGoBin() *GoBin {
	return &GoBin{
		getenv:        os.Getenv,
		userHomeDir:   os.UserHomeDir,
		readDir:       os.ReadDir,
		readBuildInfo: buildinfo.ReadFile,
	}
}

// Name implements Manager.
func (*GoBin) Name() string { return "go" }

// binDir resolves the go install target the same way the toolchain does:
// GOBIN, then the first GOPATH entry's bin, then $HOME/go/bin.
func (g *GoBin) binDir() string {
	if d := g.getenv("GOBIN"); d != "" {
		return d
	}
	if gopath := g.getenv("GOPATH"); gopath != "" {
		if first := filepath.SplitList(gopath); len(first) > 0 && first[0] != "" {
			return filepath.Join(first[0], "bin")
		}
	}
	home, err := g.userHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, "go", "bin")
}

// Detect implements Manager: a non-empty go bin directory.
func (g *GoBin) Detect() bool {
	dir := g.binDir()
	if dir == "" {
		return false
	}
	entries, err := g.readDir(dir)
	return err == nil && len(entries) > 0
}

// Audit implements Manager.
func (g *GoBin) Audit(_ context.Context) audit.ManagerReport {
	rep := audit.ManagerReport{Name: g.Name(), Detected: true}

	dir := g.binDir()
	entries, err := g.readDir(dir)
	if err != nil {
		rep.Findings = append(rep.Findings, audit.Finding{
			Severity: audit.SeverityWarning,
			Code:     audit.CodeManagerUnreadable,
			Message:  fmt.Sprintf("could not read go bin directory %s: %v", dir, err),
		})
		return rep
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		binPath := filepath.Join(dir, entry.Name())

		bi, err := g.readBuildInfo(binPath)
		if err != nil {
			// Not a Go binary (or unreadable); out of this adapter's scope.
			continue
		}

		rep.Packages = append(rep.Packages, auditGoBinary(entry.Name(), binPath, bi))
	}

	sort.Slice(rep.Packages, func(i, j int) bool { return rep.Packages[i].Name < rep.Packages[j].Name })

	return rep
}

// auditGoBinary maps one binary's embedded build metadata to a package
// entry: module origin, version, integrity, and build-hygiene findings.
func auditGoBinary(name, binPath string, bi *debug.BuildInfo) audit.SystemPackage {
	pkg := audit.SystemPackage{
		Name:       name,
		Version:    bi.Main.Version,
		Source:     bi.Main.Path,
		BinaryPath: binPath,
	}

	switch {
	case bi.Main.Version == develVersion:
		pkg.Integrity = audit.IntegrityNotVerifiable
		pkg.Findings = append(pkg.Findings, audit.Finding{
			Severity: audit.SeverityInfo,
			Code:     audit.CodeUnpinnedBuild,
			Message:  "built from a source checkout (devel), not a released module version",
		})
	case bi.Main.Sum == "":
		pkg.Integrity = audit.IntegrityNotVerifiable
		pkg.Findings = append(pkg.Findings, audit.Finding{
			Severity: audit.SeverityInfo,
			Code:     audit.CodeNoIntegrityData,
			Message:  "no module sum embedded in the binary — source integrity cannot be verified",
		})
	default:
		pkg.Integrity = audit.IntegrityRecorded
	}

	if setting(bi, "vcs.modified") == "true" {
		rev := setting(bi, "vcs.revision")
		msg := "built from a dirty VCS working tree"
		if rev != "" {
			msg = fmt.Sprintf("%s (revision %.12s + uncommitted changes)", msg, rev)
		}
		pkg.Findings = append(pkg.Findings, audit.Finding{
			Severity: audit.SeverityWarning,
			Code:     audit.CodeDirtyBuild,
			Message:  msg,
		})
	}

	return pkg
}

// setting returns a build setting value by key, or "".
func setting(bi *debug.BuildInfo, key string) string {
	for _, s := range bi.Settings {
		if s.Key == key {
			return s.Value
		}
	}
	return ""
}
