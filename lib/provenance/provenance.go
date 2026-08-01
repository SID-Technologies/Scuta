// Package provenance implements optional post-download verification
// backends for release assets: cosign keyless signatures and SLSA build
// provenance. Backends shell out to their official CLIs (cosign,
// slsa-verifier) when present on PATH, mirroring the fail-closed semantics
// of lib/fetch: material that is present but invalid always fails the
// install, while missing material or a missing verifier CLI degrades
// according to the configured mode.
package provenance

import (
	"context"
	"os/exec"
	"strings"

	"github.com/sid-technologies/scuta/lib/errors"
	"github.com/sid-technologies/scuta/lib/output"
)

// Mode controls how strictly provenance backends are applied.
type Mode string

// Provenance verification modes.
const (
	// ModeOff disables provenance verification entirely (the default).
	ModeOff Mode = "off"
	// ModeAuto verifies when both the verifier CLI and signature material
	// are available, and skips quietly otherwise. Present-but-invalid
	// material still fails the install.
	ModeAuto Mode = "auto"
	// ModeRequire fails the install unless at least one backend verifies
	// the asset.
	ModeRequire Mode = "require"
)

// ParseMode parses a provenance_verify config value. Empty means off.
func ParseMode(s string) (Mode, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", string(ModeOff):
		return ModeOff, nil
	case string(ModeAuto):
		return ModeAuto, nil
	case string(ModeRequire):
		return ModeRequire, nil
	default:
		return ModeOff, errors.New("invalid provenance mode %q (use off, auto, or require)", s)
	}
}

// Rank orders modes by strictness (off < auto < require) so callers can
// prevent a remote config from weakening a locally configured mode.
func Rank(s string) int {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case string(ModeAuto):
		return 1
	case string(ModeRequire):
		return 2
	default:
		return 0
	}
}

// Asset is a release asset visible to backends when locating companion
// signature and provenance files.
type Asset struct {
	Name string
	URL  string
}

// Request carries everything a backend needs to verify one downloaded asset.
type Request struct {
	// Repo is the "owner/name" GitHub repository the release came from.
	// Backends derive the expected signer identity / source URI from it.
	Repo string
	// Tag is the release tag (as published, e.g. "v1.2.3").
	Tag string
	// AssetName is the release asset filename being verified.
	AssetName string
	// AssetPath is the local path of the already-downloaded asset.
	AssetPath string
	// Assets lists all release assets, used to find companion material
	// (.sig, .pem, bundles, *.intoto.jsonl, signed checksums files).
	Assets []Asset
	// WorkDir is a caller-owned temp directory for companion downloads.
	WorkDir string
	// Download fetches a companion asset URL to a local path.
	Download func(ctx context.Context, url string, dest string) error
}

// Result is one backend's verdict for one asset.
type Result struct {
	// Backend is the backend name ("cosign", "slsa").
	Backend string
	// Verified is true when the backend positively verified the asset.
	Verified bool
	// Skipped is true when the backend could not run (no CLI, no material).
	Skipped bool
	// Reason explains a skip.
	Reason string
	// Detail describes the evidence used on success.
	Detail string
}

// Backend verifies a downloaded asset against release-provided material.
// A returned error means verification material was present but did not
// verify — that is always fatal regardless of mode.
type Backend interface {
	Name() string
	Verify(ctx context.Context, req Request) (Result, error)
}

// Run applies every backend to the request under the given mode.
//
// Behavior matrix (per backend):
//   - material present, verifies:      recorded as verified
//   - material present, invalid:       error (fail closed, any mode)
//   - material or CLI missing, auto:   skipped with a debug note
//   - nothing verified, require:       error listing each skip reason
func Run(ctx context.Context, mode Mode, backends []Backend, req Request) ([]Result, error) {
	if mode == ModeOff || mode == "" || len(backends) == 0 {
		return nil, nil
	}

	results := make([]Result, 0, len(backends))
	var skipReasons []string

	for _, b := range backends {
		res, err := b.Verify(ctx, req)
		if err != nil {
			return nil, errors.Wrap(err, "%s verification failed for %s", b.Name(), req.AssetName)
		}

		results = append(results, res)

		if res.Verified {
			output.Debugf("%s verified %s (%s)", b.Name(), req.AssetName, res.Detail)
			continue
		}
		if res.Skipped {
			output.Debugf("%s skipped for %s: %s", b.Name(), req.AssetName, res.Reason)
			skipReasons = append(skipReasons, b.Name()+": "+res.Reason)
		}
	}

	if mode == ModeRequire && len(VerifiedBackends(results)) == 0 {
		return nil, errors.New(
			"provenance verification required but no backend could verify %s (%s)",
			req.AssetName, strings.Join(skipReasons, "; "),
		)
	}

	return results, nil
}

// VerifiedBackends returns the names of backends that verified the asset,
// in run order. The result is suitable for recording in state.
func VerifiedBackends(results []Result) []string {
	var names []string
	for _, r := range results {
		if r.Verified {
			names = append(names, r.Backend)
		}
	}
	return names
}

// findAsset returns the release asset with the given name (case-insensitive),
// or nil when absent.
func findAsset(assets []Asset, name string) *Asset {
	for i := range assets {
		if strings.EqualFold(assets[i].Name, name) {
			return &assets[i]
		}
	}
	return nil
}

// runCommand executes a verifier CLI and returns its combined output.
//
//nolint:gosec // name is a fixed verifier binary ("cosign"/"slsa-verifier"); args are built internally
func runCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

// summarizeOutput compacts CLI output for inclusion in error messages.
func summarizeOutput(out []byte) string {
	s := strings.TrimSpace(string(out))
	s = strings.ReplaceAll(s, "\n", " | ")
	const limit = 400
	if len(s) > limit {
		s = s[:limit] + "..."
	}
	return s
}
