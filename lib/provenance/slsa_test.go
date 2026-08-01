package provenance

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestSLSA_SkipsWhenCLIMissing(t *testing.T) {
	s := &SLSA{lookPath: notFound}

	res, err := s.Verify(context.Background(), writeAsset(t, []byte("bytes")))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !res.Skipped || !strings.Contains(res.Reason, "not found") {
		t.Fatalf("expected PATH skip, got %+v", res)
	}
}

func TestSLSA_SkipsWhenNoAttestation(t *testing.T) {
	s := &SLSA{lookPath: found}
	req := writeAsset(t, []byte("bytes"))
	req.Assets = []Asset{{Name: req.AssetName}, {Name: "checksums.txt"}}

	res, err := s.Verify(context.Background(), req)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !res.Skipped || !strings.Contains(res.Reason, "no provenance attestation") {
		t.Fatalf("expected attestation skip, got %+v", res)
	}
}

func TestSLSA_HappyPath(t *testing.T) {
	runner := &recordingRunner{out: []byte("PASSED")}
	s := &SLSA{lookPath: found, run: runner.run}

	req := writeAsset(t, []byte("bytes"))
	req.Assets = []Asset{
		{Name: req.AssetName},
		{Name: "multiple.intoto.jsonl", URL: "https://example.com/prov"},
	}
	req.Download = stubDownload(t, map[string][]byte{
		"https://example.com/prov": []byte("{}"),
	})

	res, err := s.Verify(context.Background(), req)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !res.Verified {
		t.Fatalf("expected verified, got %+v", res)
	}

	if runner.name != "slsa-verifier" || runner.args[0] != "verify-artifact" {
		t.Fatalf("unexpected invocation: %s %v", runner.name, runner.args)
	}
	if runner.args[1] != req.AssetPath {
		t.Fatalf("artifact should be the asset path: %v", runner.args)
	}
	if !hasArgPair(runner.args, "--source-uri", "github.com/owner/tool") {
		t.Fatalf("missing --source-uri: %v", runner.args)
	}
	if !hasArgPair(runner.args, "--source-tag", "v1.0.0") {
		t.Fatalf("missing --source-tag: %v", runner.args)
	}
}

func TestSLSA_PrefersAssetSpecificAttestation(t *testing.T) {
	runner := &recordingRunner{}
	s := &SLSA{lookPath: found, run: runner.run}

	req := writeAsset(t, []byte("bytes"))
	specific := req.AssetName + ".intoto.jsonl"
	req.Assets = []Asset{
		{Name: req.AssetName},
		{Name: "multiple.intoto.jsonl", URL: "https://example.com/multi"},
		{Name: specific, URL: "https://example.com/specific"},
	}
	req.Download = stubDownload(t, map[string][]byte{
		"https://example.com/specific": []byte("{}"),
	})

	res, err := s.Verify(context.Background(), req)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !strings.Contains(res.Detail, specific) {
		t.Fatalf("expected asset-specific attestation, got %+v", res)
	}
}

func TestSLSA_AmbiguousAttestationsSkip(t *testing.T) {
	s := &SLSA{lookPath: found}
	req := writeAsset(t, []byte("bytes"))
	req.Assets = []Asset{
		{Name: req.AssetName},
		{Name: "one.intoto.jsonl"},
		{Name: "two.intoto.jsonl"},
	}

	res, err := s.Verify(context.Background(), req)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !res.Skipped || !strings.Contains(res.Reason, "multiple provenance") {
		t.Fatalf("expected ambiguity skip, got %+v", res)
	}
}

func TestSLSA_RejectionIsFatal(t *testing.T) {
	runner := &recordingRunner{out: []byte("FAILED: untrusted builder"), err: errors.New("exit status 2")}
	s := &SLSA{lookPath: found, run: runner.run}

	req := writeAsset(t, []byte("bytes"))
	req.Assets = []Asset{
		{Name: req.AssetName},
		{Name: "multiple.intoto.jsonl", URL: "https://example.com/prov"},
	}
	req.Download = stubDownload(t, map[string][]byte{
		"https://example.com/prov": []byte("{}"),
	})

	_, err := s.Verify(context.Background(), req)
	if err == nil || !strings.Contains(err.Error(), "slsa-verifier rejected") {
		t.Fatalf("expected rejection error, got %v", err)
	}
}

func TestSLSA_NoRepoSkips(t *testing.T) {
	s := &SLSA{lookPath: found}
	req := writeAsset(t, []byte("bytes"))
	req.Repo = ""
	req.Assets = []Asset{
		{Name: req.AssetName},
		{Name: "multiple.intoto.jsonl"},
	}

	res, err := s.Verify(context.Background(), req)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !res.Skipped || !strings.Contains(res.Reason, "source repository") {
		t.Fatalf("expected repo skip, got %+v", res)
	}
}
