package audit

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/sid-technologies/scuta/lib/policy"
	"github.com/sid-technologies/scuta/lib/state"
)

// writeBinary creates an executable file and returns its path and SHA-256.
func writeBinary(t *testing.T, dir, name, content string) (string, string) {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o755); err != nil { //nolint:gosec // test fixture
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(content))
	return p, hex.EncodeToString(sum[:])
}

func findingCodes(fs []Finding) []string {
	codes := make([]string, 0, len(fs))
	for _, f := range fs {
		codes = append(codes, f.Code)
	}
	return codes
}

func hasCode(fs []Finding, code string) bool {
	for _, f := range fs {
		if f.Code == code {
			return true
		}
	}
	return false
}

func TestAuditTool_CleanVerifiedInstall(t *testing.T) {
	dir := t.TempDir()
	bin, sha := writeBinary(t, dir, "tool", "binary-contents")

	tool := auditTool("tool", state.ToolState{
		Version:    "1.0.0",
		BinaryPath: bin,
		Sha256:     sha,
		Verified:   true,
	}, nil)

	if len(tool.Findings) != 0 {
		t.Fatalf("expected no findings, got %v", findingCodes(tool.Findings))
	}
	if !tool.Present || !tool.Executable || tool.Drift {
		t.Fatalf("unexpected flags: present=%v executable=%v drift=%v", tool.Present, tool.Executable, tool.Drift)
	}
	if tool.CurrentSha256 != sha {
		t.Fatalf("expected current sha %s, got %s", sha, tool.CurrentSha256)
	}
}

func TestAuditTool_BinaryDrift(t *testing.T) {
	dir := t.TempDir()
	bin, _ := writeBinary(t, dir, "tool", "tampered-contents")

	tool := auditTool("tool", state.ToolState{
		Version:    "1.0.0",
		BinaryPath: bin,
		Sha256:     "0000000000000000000000000000000000000000000000000000000000000000",
		Verified:   true,
	}, nil)

	if !tool.Drift {
		t.Fatal("expected drift to be detected")
	}
	if !hasCode(tool.Findings, CodeBinaryDrift) {
		t.Fatalf("expected %s finding, got %v", CodeBinaryDrift, findingCodes(tool.Findings))
	}
}

func TestAuditTool_MissingBinary(t *testing.T) {
	tool := auditTool("tool", state.ToolState{
		Version:    "1.0.0",
		BinaryPath: filepath.Join(t.TempDir(), "does-not-exist"),
	}, nil)

	if tool.Present {
		t.Fatal("expected present=false")
	}
	if !hasCode(tool.Findings, CodeMissingBinary) {
		t.Fatalf("expected %s finding, got %v", CodeMissingBinary, findingCodes(tool.Findings))
	}
}

func TestAuditTool_UnknownProvenance(t *testing.T) {
	dir := t.TempDir()
	bin, _ := writeBinary(t, dir, "tool", "legacy-install")

	tool := auditTool("tool", state.ToolState{
		Version:    "1.0.0",
		BinaryPath: bin,
		// No Sha256: installed by an older scuta.
	}, nil)

	if !hasCode(tool.Findings, CodeUnknownProvenance) {
		t.Fatalf("expected %s finding, got %v", CodeUnknownProvenance, findingCodes(tool.Findings))
	}
	if tool.Drift {
		t.Fatal("legacy installs must not be reported as drift")
	}
}

func TestAuditTool_UnverifiedInstall(t *testing.T) {
	dir := t.TempDir()
	bin, sha := writeBinary(t, dir, "tool", "unverified")

	tool := auditTool("tool", state.ToolState{
		Version:    "1.0.0",
		BinaryPath: bin,
		Sha256:     sha,
		Verified:   false,
	}, nil)

	if !hasCode(tool.Findings, CodeUnverifiedInstall) {
		t.Fatalf("expected %s finding, got %v", CodeUnverifiedInstall, findingCodes(tool.Findings))
	}
}

func TestAuditTool_PolicyViolation(t *testing.T) {
	dir := t.TempDir()
	bin, sha := writeBinary(t, dir, "tool", "contents")

	pol, err := policy.Parse([]byte("tools:\n  tool:\n    allowed: \">=2.0.0\"\n"))
	if err != nil {
		t.Fatal(err)
	}

	tool := auditTool("tool", state.ToolState{
		Version:    "1.0.0",
		BinaryPath: bin,
		Sha256:     sha,
		Verified:   true,
	}, pol)

	if !hasCode(tool.Findings, CodePolicyViolation) {
		t.Fatalf("expected %s finding, got %v", CodePolicyViolation, findingCodes(tool.Findings))
	}
}

func TestAuditTools_SortedByName(t *testing.T) {
	dir := t.TempDir()
	binA, shaA := writeBinary(t, dir, "alpha", "a")
	binZ, shaZ := writeBinary(t, dir, "zeta", "z")

	st := state.NewState()
	st.SetTool("zeta", state.ToolState{Version: "1.0.0", BinaryPath: binZ, Sha256: shaZ, Verified: true})
	st.SetTool("alpha", state.ToolState{Version: "1.0.0", BinaryPath: binA, Sha256: shaA, Verified: true})

	tools := CheckTools(st, nil)
	if len(tools) != 2 || tools[0].Name != "alpha" || tools[1].Name != "zeta" {
		t.Fatalf("expected sorted [alpha zeta], got %v", tools)
	}
}

func TestAuditPosture(t *testing.T) {
	// Nothing configured: no-trust-root warning + no-policy info.
	p := CheckPosture(PostureInput{})
	if !hasCode(p.Findings, CodeNoTrustRoot) || !hasCode(p.Findings, CodeNoPolicy) {
		t.Fatalf("expected no-trust-root and no-policy, got %v", findingCodes(p.Findings))
	}

	// Trust root without required signed metadata: unsigned-metadata warning.
	p = CheckPosture(PostureInput{TrustRootConfigured: true, PolicyConfigured: true})
	if !hasCode(p.Findings, CodeUnsignedMetadata) {
		t.Fatalf("expected %s, got %v", CodeUnsignedMetadata, findingCodes(p.Findings))
	}

	// Fully hardened: no findings.
	p = CheckPosture(PostureInput{TrustRootConfigured: true, RequireSignedMetadata: true, PolicyConfigured: true})
	if len(p.Findings) != 0 {
		t.Fatalf("expected no findings, got %v", findingCodes(p.Findings))
	}
}

func TestFinalizeCounts(t *testing.T) {
	r := New("test")
	r.Posture = Posture{Findings: []Finding{
		{Severity: SeverityWarning, Code: CodeNoTrustRoot},
		{Severity: SeverityInfo, Code: CodeNoPolicy},
	}}
	r.Tools = []Tool{
		{Name: "a", Findings: []Finding{{Severity: SeverityCritical, Code: CodeBinaryDrift}}},
		{Name: "b"},
	}
	r.Finalize()

	if r.Summary.Tools != 2 || r.Summary.Criticals != 1 || r.Summary.Warnings != 1 {
		t.Fatalf("unexpected summary: %+v", r.Summary)
	}
	if r.GeneratedAt.After(time.Now()) {
		t.Fatal("GeneratedAt in the future")
	}
}

func TestIsExecutable_WindowsHasNoExecBits(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "tool")
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}

	got := isExecutable(info)
	if runtime.GOOS == "windows" {
		// Windows reports no POSIX exec bits on regular files; presence
		// must be sufficient or every audited tool goes critical.
		if !got {
			t.Fatal("expected executable=true on windows")
		}
	} else if got {
		t.Fatal("expected executable=false for 0o644 on POSIX")
	}
}

func TestAuditTool_ProvenanceSurfacedInReport(t *testing.T) {
	dir := t.TempDir()
	bin, sha := writeBinary(t, dir, "tool", "#!/bin/sh\n")

	st := state.NewState()
	st.SetTool("tool", state.ToolState{
		Version:    "1.0.0",
		BinaryPath: bin,
		Sha256:     sha,
		Verified:   true,
		Provenance: []string{"cosign"},
	})

	tools := CheckTools(st, nil)
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	if len(tools[0].Provenance) != 1 || tools[0].Provenance[0] != "cosign" {
		t.Fatalf("Provenance = %v, want [cosign]", tools[0].Provenance)
	}
	for _, f := range tools[0].Findings {
		if f.Severity == SeverityCritical {
			t.Fatalf("unexpected critical finding: %+v", f)
		}
	}
}
