package managers

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/sid-technologies/scuta/lib/audit"
)

// maxDpkgFindings caps how many file-level verification failures are
// reported before summarizing, keeping reports bounded on drifted systems.
const maxDpkgFindings = 25

// dpkgStatusPath is the dpkg database; its presence defines a dpkg system.
const dpkgStatusPath = "/var/lib/dpkg/status"

// Dpkg audits packages installed by dpkg/apt. Unlike brew, dpkg stores
// per-file md5sums, so real (if weak) drift detection is possible with a
// single `dpkg --verify` run. Packages are counted, not listed — Debian
// systems carry thousands and only verification failures are interesting.
type Dpkg struct {
	stat     func(string) (os.FileInfo, error)
	readFile func(string) ([]byte, error)
	lookPath func(string) (string, error)
	run      func(ctx context.Context, name string, args ...string) ([]byte, error)
}

// NewDpkg returns a Dpkg backed by the real OS.
func NewDpkg() *Dpkg {
	return &Dpkg{
		stat:     os.Stat,
		readFile: os.ReadFile,
		lookPath: exec.LookPath,
		run: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			// dpkg --verify exits non-zero when discrepancies exist;
			// that is data, not an execution failure.
			out, err := exec.CommandContext(ctx, name, args...).Output()
			var exitErr *exec.ExitError
			if err != nil && errors.As(err, &exitErr) {
				return out, nil
			}
			return out, err
		},
	}
}

// Name implements Manager.
func (*Dpkg) Name() string { return "dpkg" }

// Detect implements Manager.
func (d *Dpkg) Detect() bool {
	_, err := d.stat(dpkgStatusPath)
	return err == nil
}

// Audit implements Manager.
func (d *Dpkg) Audit(ctx context.Context) audit.ManagerReport {
	rep := audit.ManagerReport{Name: d.Name(), Detected: true}

	installed := d.countInstalled()
	rep.Findings = append(rep.Findings, audit.Finding{
		Severity: audit.SeverityInfo,
		Code:     audit.CodeInventorySummarized,
		Message:  fmt.Sprintf("%d package(s) installed; only verification failures are listed", installed),
	})

	dpkg, err := d.lookPath("dpkg")
	if err != nil {
		rep.Findings = append(rep.Findings, audit.Finding{
			Severity: audit.SeverityWarning,
			Code:     audit.CodeManagerUnreadable,
			Message:  "dpkg database present but dpkg binary not found — cannot verify",
		})
		return rep
	}

	out, err := d.run(ctx, dpkg, "--verify")
	if err != nil {
		rep.Findings = append(rep.Findings, audit.Finding{
			Severity: audit.SeverityWarning,
			Code:     audit.CodeManagerUnreadable,
			Message:  fmt.Sprintf("dpkg --verify failed: %v", err),
		})
		return rep
	}

	rep.Findings = append(rep.Findings, parseDpkgVerify(out)...)

	return rep
}

// countInstalled counts "Package:" stanzas in the dpkg status database.
func (d *Dpkg) countInstalled() int {
	data, err := d.readFile(dpkgStatusPath)
	if err != nil {
		return 0
	}
	return bytes.Count(data, []byte("\nPackage: ")) + boolToInt(bytes.HasPrefix(data, []byte("Package: ")))
}

// parseDpkgVerify maps dpkg --verify output lines to findings. Lines look
// like "??5??????   /usr/bin/tool" (md5 mismatch) or "missing   /etc/x".
// Changed conffiles are expected churn and reported as info; changed
// non-config files are drift and reported as critical.
func parseDpkgVerify(out []byte) []audit.Finding {
	var findings []audit.Finding
	total := 0

	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		total++
		if len(findings) >= maxDpkgFindings {
			continue
		}

		conffile := strings.Contains(line, " c ")
		severity := audit.SeverityCritical
		code := audit.CodeBinaryDrift
		if conffile {
			severity = audit.SeverityInfo
			code = audit.CodeConfigDrift
		}
		findings = append(findings, audit.Finding{
			Severity: severity,
			Code:     code,
			Message:  "dpkg verify: " + line,
		})
	}

	if total > maxDpkgFindings {
		findings = append(findings, audit.Finding{
			Severity: audit.SeverityWarning,
			Code:     audit.CodeInventorySummarized,
			Message:  fmt.Sprintf("%d further verification failure(s) not listed", total-maxDpkgFindings),
		})
	}

	return findings
}

// boolToInt is a tiny helper for the stanza count above.
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
