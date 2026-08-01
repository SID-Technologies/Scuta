package installer

import (
	"context"
	"strings"
	"testing"

	"github.com/sid-technologies/scuta/lib/github"
	"github.com/sid-technologies/scuta/lib/provenance"
)

// stubProvBackend is a canned-response provenance backend.
type stubProvBackend struct {
	res provenance.Result
	err error
}

func (s *stubProvBackend) Name() string { return s.res.Backend }
func (s *stubProvBackend) Verify(context.Context, provenance.Request) (provenance.Result, error) {
	return s.res, s.err
}

func TestVerifyProvenance_OffModeIsNoOp(t *testing.T) {
	inst := New(github.NewClient(""), t.TempDir())

	got, err := inst.VerifyProvenance(context.Background(), &github.Release{TagName: "v1.0.0"}, "owner/tool", "a.tar.gz", "/nonexistent", false)
	if err != nil {
		t.Fatalf("VerifyProvenance: %v", err)
	}
	if got != nil {
		t.Fatalf("off mode must verify nothing, got %v", got)
	}
}

func TestVerifyProvenance_SkipVerifyRejectedUnderRequire(t *testing.T) {
	inst := New(github.NewClient(""), t.TempDir())
	inst.provenanceMode = provenance.ModeRequire
	inst.provenanceBackends = []provenance.Backend{
		&stubProvBackend{res: provenance.Result{Backend: "cosign", Verified: true}},
	}

	_, err := inst.VerifyProvenance(context.Background(), &github.Release{TagName: "v1.0.0"}, "owner/tool", "a.tar.gz", "/nonexistent", true)
	if err == nil || !strings.Contains(err.Error(), "--skip-verify") {
		t.Fatalf("expected --skip-verify rejection under require, got %v", err)
	}
}

func TestVerifyProvenance_SkipVerifySkipsUnderAuto(t *testing.T) {
	inst := New(github.NewClient(""), t.TempDir())
	inst.provenanceMode = provenance.ModeAuto
	inst.provenanceBackends = []provenance.Backend{
		&stubProvBackend{res: provenance.Result{Backend: "cosign", Verified: true}},
	}

	got, err := inst.VerifyProvenance(context.Background(), &github.Release{TagName: "v1.0.0"}, "owner/tool", "a.tar.gz", "/nonexistent", true)
	if err != nil {
		t.Fatalf("VerifyProvenance: %v", err)
	}
	if got != nil {
		t.Fatalf("--skip-verify under auto must skip backends, got %v", got)
	}
}

func TestVerifyProvenance_RecordsVerifiedBackends(t *testing.T) {
	inst := New(github.NewClient(""), t.TempDir())
	inst.provenanceMode = provenance.ModeAuto
	inst.provenanceBackends = []provenance.Backend{
		&stubProvBackend{res: provenance.Result{Backend: "cosign", Verified: true}},
		&stubProvBackend{res: provenance.Result{Backend: "slsa", Skipped: true, Reason: "no CLI"}},
	}

	got, err := inst.VerifyProvenance(context.Background(), &github.Release{TagName: "v1.0.0"}, "owner/tool", "a.tar.gz", "/nonexistent", false)
	if err != nil {
		t.Fatalf("VerifyProvenance: %v", err)
	}
	if len(got) != 1 || got[0] != "cosign" {
		t.Fatalf("got %v, want [cosign]", got)
	}
}

func TestSetProvenanceVerification(t *testing.T) {
	inst := New(github.NewClient(""), t.TempDir())

	inst.SetProvenanceVerification(provenance.ModeAuto, provenance.CosignIdentity{})
	if inst.provenanceMode != provenance.ModeAuto || len(inst.provenanceBackends) != 2 {
		t.Fatalf("expected auto mode with 2 backends, got %q / %d", inst.provenanceMode, len(inst.provenanceBackends))
	}

	inst.SetProvenanceVerification(provenance.ModeOff, provenance.CosignIdentity{})
	if inst.provenanceBackends != nil {
		t.Fatal("off mode must clear backends")
	}
}
