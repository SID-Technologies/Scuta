package provenance

import (
	"context"
	"os/exec"
	"strings"

	"github.com/sid-technologies/scuta/lib/errors"
)

// SLSA verifies release assets against SLSA build provenance attestations
// (*.intoto.jsonl) using the slsa-verifier CLI, checking that the asset was
// built by the source repository's release workflow.
type SLSA struct {
	// lookPath and run are injectable for tests.
	lookPath func(file string) (string, error)
	run      func(ctx context.Context, name string, args ...string) ([]byte, error)
}

// NewSLSA returns a SLSA backend using the slsa-verifier CLI from PATH.
func NewSLSA() *SLSA {
	return &SLSA{
		lookPath: exec.LookPath,
		run:      runCommand,
	}
}

// Name implements Backend.
func (*SLSA) Name() string { return "slsa" }

// Verify implements Backend.
func (s *SLSA) Verify(ctx context.Context, req Request) (Result, error) {
	skip := func(reason string) Result {
		return Result{Backend: s.Name(), Skipped: true, Reason: reason}
	}

	if _, err := s.lookPath("slsa-verifier"); err != nil {
		return skip("slsa-verifier not found in PATH"), nil
	}

	prov, ambiguous := findProvenanceAsset(req.Assets, req.AssetName)
	if ambiguous {
		return skip("multiple provenance attestations in release, none matching the asset"), nil
	}
	if prov == nil {
		return skip("no provenance attestation (*.intoto.jsonl) in release"), nil
	}

	if req.Repo == "" {
		return skip("cannot derive source repository for provenance check"), nil
	}

	provPath, err := downloadCompanion(ctx, req, *prov)
	if err != nil {
		return Result{}, err
	}

	args := []string{
		"verify-artifact", req.AssetPath,
		"--provenance-path", provPath,
		"--source-uri", "github.com/" + req.Repo,
	}
	if req.Tag != "" {
		args = append(args, "--source-tag", req.Tag)
	}

	out, err := s.run(ctx, "slsa-verifier", args...)
	if err != nil {
		return Result{}, errors.New("slsa-verifier rejected %s: %v: %s", req.AssetName, err, summarizeOutput(out))
	}

	return Result{
		Backend:  s.Name(),
		Verified: true,
		Detail:   "attestation " + prov.Name,
	}, nil
}

// findProvenanceAsset picks the provenance attestation covering assetName.
// Preference order: an attestation named after the asset, the GoReleaser
// aggregate "multiple.intoto.jsonl", then a sole remaining attestation.
// It reports ambiguity when several unrelated attestations exist.
func findProvenanceAsset(assets []Asset, assetName string) (found *Asset, ambiguous bool) {
	if a := findAsset(assets, assetName+".intoto.jsonl"); a != nil {
		return a, false
	}
	if a := findAsset(assets, "multiple.intoto.jsonl"); a != nil {
		return a, false
	}

	var candidates []*Asset
	for i := range assets {
		if strings.HasSuffix(strings.ToLower(assets[i].Name), ".intoto.jsonl") {
			candidates = append(candidates, &assets[i])
		}
	}

	switch len(candidates) {
	case 0:
		return nil, false
	case 1:
		return candidates[0], false
	default:
		return nil, true
	}
}
