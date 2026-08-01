package provenance

import (
	"context"
	"testing"
)

func TestParseMode(t *testing.T) {
	cases := []struct {
		in      string
		want    Mode
		wantErr bool
	}{
		{"", ModeOff, false},
		{"off", ModeOff, false},
		{"auto", ModeAuto, false},
		{"require", ModeRequire, false},
		{"REQUIRE", ModeRequire, false},
		{" auto ", ModeAuto, false},
		{"strict", ModeOff, true},
	}

	for _, tc := range cases {
		got, err := ParseMode(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("ParseMode(%q): expected error", tc.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseMode(%q): %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("ParseMode(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestRank(t *testing.T) {
	if Rank("off") >= Rank("auto") || Rank("auto") >= Rank("require") {
		t.Fatal("expected off < auto < require")
	}
	if Rank("") != Rank("off") {
		t.Fatal("empty mode should rank as off")
	}
}

// stubBackend is a canned-response backend for orchestrator tests.
type stubBackend struct {
	name string
	res  Result
	err  error
}

func (s *stubBackend) Name() string { return s.name }
func (s *stubBackend) Verify(context.Context, Request) (Result, error) {
	return s.res, s.err
}

func TestRun_OffMode(t *testing.T) {
	backends := []Backend{&stubBackend{name: "x", res: Result{Backend: "x", Verified: true}}}

	results, err := Run(context.Background(), ModeOff, backends, Request{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if results != nil {
		t.Fatal("off mode must not run any backend")
	}
}

func TestRun_AutoSkippedIsOK(t *testing.T) {
	backends := []Backend{
		&stubBackend{name: "cosign", res: Result{Backend: "cosign", Skipped: true, Reason: "no material"}},
		&stubBackend{name: "slsa", res: Result{Backend: "slsa", Skipped: true, Reason: "no CLI"}},
	}

	results, err := Run(context.Background(), ModeAuto, backends, Request{AssetName: "a.tar.gz"})
	if err != nil {
		t.Fatalf("auto mode with skips must not error: %v", err)
	}
	if got := VerifiedBackends(results); got != nil {
		t.Fatalf("expected no verified backends, got %v", got)
	}
}

func TestRun_RequireFailsWhenNothingVerifies(t *testing.T) {
	backends := []Backend{
		&stubBackend{name: "cosign", res: Result{Backend: "cosign", Skipped: true, Reason: "no material"}},
	}

	_, err := Run(context.Background(), ModeRequire, backends, Request{AssetName: "a.tar.gz"})
	if err == nil {
		t.Fatal("require mode must fail when no backend verifies")
	}
}

func TestRun_RequireSucceedsWhenOneVerifies(t *testing.T) {
	backends := []Backend{
		&stubBackend{name: "cosign", res: Result{Backend: "cosign", Skipped: true, Reason: "no material"}},
		&stubBackend{name: "slsa", res: Result{Backend: "slsa", Verified: true}},
	}

	results, err := Run(context.Background(), ModeRequire, backends, Request{AssetName: "a.tar.gz"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	got := VerifiedBackends(results)
	if len(got) != 1 || got[0] != "slsa" {
		t.Fatalf("VerifiedBackends = %v, want [slsa]", got)
	}
}

func TestRun_InvalidMaterialIsFatalInAutoMode(t *testing.T) {
	backends := []Backend{
		&stubBackend{name: "cosign", err: errString("signature does not match")},
	}

	_, err := Run(context.Background(), ModeAuto, backends, Request{AssetName: "a.tar.gz"})
	if err == nil {
		t.Fatal("present-but-invalid material must fail even in auto mode")
	}
}

// errString is a trivial error type for stubs.
type errString string

func (e errString) Error() string { return string(e) }
