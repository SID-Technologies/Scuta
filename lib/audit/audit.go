// Package audit builds a machine-readable security posture report for the
// local scuta installation: per-tool provenance and drift checks plus
// machine-level configuration posture. It is the read-only reporting half of
// the org story — `sync --check` gates CI, `doctor --audit` reports.
package audit

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"runtime"
	"sort"
	"time"

	"github.com/sid-technologies/scuta/lib/policy"
	"github.com/sid-technologies/scuta/lib/state"
)

// Finding severities. Critical findings should fail CI; warnings are
// advisory; info is context.
const (
	SeverityCritical = "critical"
	SeverityWarning  = "warning"
	SeverityInfo     = "info"
)

// Finding codes, stable identifiers for fleet aggregation.
const (
	CodeMissingBinary      = "missing-binary"
	CodeNotExecutable      = "not-executable"
	CodeBinaryDrift        = "binary-drift"
	CodeUnknownProvenance  = "unknown-provenance"
	CodeUnverifiedInstall  = "unverified-install"
	CodePolicyViolation    = "policy-violation"
	CodeKnownVulnerability = "known-vulnerability"
	CodeNoTrustRoot        = "no-trust-root"
	CodeUnsignedMetadata   = "unsigned-metadata-allowed"
	CodeNoPolicy           = "no-policy"
)

// SchemaVersion identifies the JSON report format.
const SchemaVersion = 1

// Finding is a single audit observation.
type Finding struct {
	Severity string `json:"severity"`
	Code     string `json:"code"`
	Message  string `json:"message"`
}

// Tool is the audit result for one installed tool.
type Tool struct {
	Name          string    `json:"name"`
	Version       string    `json:"version"`
	Repo          string    `json:"repo,omitempty"`
	BinaryPath    string    `json:"binary_path"`
	InstalledAt   time.Time `json:"installed_at"`
	Present       bool      `json:"present"`
	Executable    bool      `json:"executable"`
	Verified      bool      `json:"verified"`
	Provenance    []string  `json:"provenance,omitempty"`
	Sha256        string    `json:"sha256,omitempty"`
	CurrentSha256 string    `json:"current_sha256,omitempty"`
	Drift         bool      `json:"drift"`
	Findings      []Finding `json:"findings,omitempty"`
}

// Posture is the machine-level security configuration snapshot.
type Posture struct {
	TrustRootConfigured   bool      `json:"trust_root_configured"`
	RequireSignature      bool      `json:"require_signature"`
	RequireSignedMetadata bool      `json:"require_signed_metadata"`
	PolicyConfigured      bool      `json:"policy_configured"`
	ConfigURL             string    `json:"config_url,omitempty"`
	Findings              []Finding `json:"findings,omitempty"`
}

// Summary aggregates finding counts across the report.
type Summary struct {
	Tools     int `json:"tools"`
	Criticals int `json:"criticals"`
	Warnings  int `json:"warnings"`
}

// Report is the full audit output.
type Report struct {
	SchemaVersion int       `json:"schema_version"`
	GeneratedAt   time.Time `json:"generated_at"`
	Hostname      string    `json:"hostname,omitempty"`
	ScutaVersion  string    `json:"scuta_version"`
	OS            string    `json:"os"`
	Arch          string    `json:"arch"`
	Posture       Posture   `json:"posture"`
	Tools         []Tool    `json:"tools"`
	Summary       Summary   `json:"summary"`
}

// PostureInput carries the resolved configuration needed for posture checks,
// so this package does not depend on config loading.
type PostureInput struct {
	TrustRootConfigured   bool
	RequireSignature      bool
	RequireSignedMetadata bool
	PolicyConfigured      bool
	ConfigURL             string
}

// New builds a Report skeleton with environment metadata.
func New(scutaVersion string) *Report {
	hostname, _ := os.Hostname()

	return &Report{
		SchemaVersion: SchemaVersion,
		GeneratedAt:   time.Now().UTC(),
		Hostname:      hostname,
		ScutaVersion:  scutaVersion,
		OS:            runtime.GOOS,
		Arch:          runtime.GOARCH,
	}
}

// CheckPosture evaluates machine-level configuration posture.
func CheckPosture(in PostureInput) Posture {
	p := Posture{
		TrustRootConfigured:   in.TrustRootConfigured,
		RequireSignature:      in.RequireSignature,
		RequireSignedMetadata: in.RequireSignedMetadata,
		PolicyConfigured:      in.PolicyConfigured,
		ConfigURL:             in.ConfigURL,
	}

	if !in.TrustRootConfigured {
		p.Findings = append(p.Findings, Finding{
			Severity: SeverityWarning,
			Code:     CodeNoTrustRoot,
			Message:  "no signature_public_key configured — release and metadata signatures cannot be verified",
		})
	}

	if in.TrustRootConfigured && !in.RequireSignedMetadata {
		p.Findings = append(p.Findings, Finding{
			Severity: SeverityWarning,
			Code:     CodeUnsignedMetadata,
			Message:  "trust root is configured but require_signed_metadata is off — unsigned registry/policy/config are still accepted",
		})
	}

	if !in.PolicyConfigured {
		p.Findings = append(p.Findings, Finding{
			Severity: SeverityInfo,
			Code:     CodeNoPolicy,
			Message:  "no policy configured — version constraints are not enforced",
		})
	}

	return p
}

// CheckTools evaluates every installed tool from state against the policy.
// Results are sorted by tool name for stable output.
func CheckTools(st *state.State, pol *policy.Policy) []Tool {
	if st == nil || len(st.Tools) == 0 {
		return nil
	}

	tools := make([]Tool, 0, len(st.Tools))
	for name, ts := range st.Tools {
		tools = append(tools, auditTool(name, ts, pol))
	}

	sort.Slice(tools, func(i, j int) bool { return tools[i].Name < tools[j].Name })

	return tools
}

// auditTool runs the per-tool checks: presence, executability, provenance,
// drift, and policy compliance.
func auditTool(name string, ts state.ToolState, pol *policy.Policy) Tool {
	t := Tool{
		Name:        name,
		Version:     ts.Version,
		Repo:        ts.Repo,
		BinaryPath:  ts.BinaryPath,
		InstalledAt: ts.InstalledAt,
		Verified:    ts.Verified,
		Provenance:  ts.Provenance,
		Sha256:      ts.Sha256,
	}

	info, err := os.Stat(ts.BinaryPath)
	if err != nil {
		t.Findings = append(t.Findings, Finding{
			Severity: SeverityCritical,
			Code:     CodeMissingBinary,
			Message:  fmt.Sprintf("binary not found at %s", ts.BinaryPath),
		})
		return t
	}
	t.Present = true

	t.Executable = isExecutable(info)
	if !t.Executable {
		t.Findings = append(t.Findings, Finding{
			Severity: SeverityCritical,
			Code:     CodeNotExecutable,
			Message:  fmt.Sprintf("binary at %s is not executable", ts.BinaryPath),
		})
	}

	auditProvenance(&t, ts)

	if v := pol.CheckToolVersion(name, ts.Version); v != nil {
		t.Findings = append(t.Findings, Finding{
			Severity: SeverityCritical,
			Code:     CodePolicyViolation,
			Message:  v.Message,
		})
	}

	return t
}

// auditProvenance checks the recorded install hash against the binary on
// disk. Tools installed before hashes were recorded are reported as unknown
// provenance rather than failed.
func auditProvenance(t *Tool, ts state.ToolState) {
	if ts.Sha256 == "" {
		t.Findings = append(t.Findings, Finding{
			Severity: SeverityWarning,
			Code:     CodeUnknownProvenance,
			Message:  "no install hash recorded (installed by an older scuta) — reinstall to enable tamper detection",
		})
		return
	}

	current, err := fileSHA256(ts.BinaryPath)
	if err != nil {
		t.Findings = append(t.Findings, Finding{
			Severity: SeverityWarning,
			Code:     CodeUnknownProvenance,
			Message:  fmt.Sprintf("could not hash binary: %v", err),
		})
		return
	}
	t.CurrentSha256 = current

	if current != ts.Sha256 {
		t.Drift = true
		t.Findings = append(t.Findings, Finding{
			Severity: SeverityCritical,
			Code:     CodeBinaryDrift,
			Message:  "binary on disk does not match the hash recorded at install time — modified outside scuta",
		})
		return
	}

	if !ts.Verified {
		t.Findings = append(t.Findings, Finding{
			Severity: SeverityWarning,
			Code:     CodeUnverifiedInstall,
			Message:  "installed without upstream checksum verification",
		})
	}
}

// Finalize recomputes the summary counts. Call after all findings
// (including caller-added ones such as CVE results) are attached.
func (r *Report) Finalize() {
	r.Summary = Summary{Tools: len(r.Tools)}

	count := func(fs []Finding) {
		for _, f := range fs {
			switch f.Severity {
			case SeverityCritical:
				r.Summary.Criticals++
			case SeverityWarning:
				r.Summary.Warnings++
			default:
				// info findings are not counted
			}
		}
	}

	count(r.Posture.Findings)
	for i := range r.Tools {
		count(r.Tools[i].Findings)
	}
}

// isExecutable reports whether the file should be considered executable.
// Windows has no POSIX execute bits (Go reports none on regular files),
// so presence is sufficient there; executability is determined by the
// .exe extension, which scuta controls at install time.
func isExecutable(info os.FileInfo) bool {
	if runtime.GOOS == "windows" {
		return true
	}
	return info.Mode()&0o111 != 0
}

// fileSHA256 returns the lowercase hex SHA-256 of the file at path.
func fileSHA256(path string) (string, error) {
	f, err := os.Open(path) //nolint:gosec // path comes from scuta-managed state
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}
