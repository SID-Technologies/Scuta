package cmd

import (
	"github.com/sid-technologies/scuta/lib/config"
	"github.com/sid-technologies/scuta/lib/github"
	"github.com/sid-technologies/scuta/lib/installer"
	"github.com/sid-technologies/scuta/lib/output"
	"github.com/sid-technologies/scuta/lib/provenance"
)

// newGitHubClient creates a GitHub client with token and optional base URL from config.
func newGitHubClient(token string, scutaDir string) *github.Client {
	client := github.NewClient(token)

	cfg, err := config.Load(scutaDir)
	if err != nil {
		return client
	}

	if cfg.GithubBaseURL != "" {
		client.SetBaseURL(cfg.GithubBaseURL)
		output.Debugf("Using GitHub base URL: %s", cfg.GithubBaseURL)
	}

	return client
}

// applyDownloadCacheConfig disables the installer download cache when the
// disable_download_cache config key is set. The cache is on by default.
func applyDownloadCacheConfig(inst *installer.Installer, scutaDir string) {
	cfg, err := config.LoadWithMerge(scutaDir)
	if err == nil && cfg.DisableDownloadCache {
		inst.SetDownloadCache(false)
		output.Debugf("Download cache disabled via config")
	}
}

// applyProvenanceConfig wires the optional provenance backends (cosign,
// slsa-verifier) onto the installer according to the provenance_verify
// config key. Off by default; a bad value is warned about, not fatal.
func applyProvenanceConfig(inst *installer.Installer, scutaDir string) {
	cfg, err := config.LoadWithMerge(scutaDir)
	if err != nil {
		return
	}

	mode, err := provenance.ParseMode(cfg.ProvenanceVerify)
	if err != nil {
		output.Warning("Ignoring provenance_verify: %v", err)
		return
	}
	if mode == provenance.ModeOff {
		return
	}

	inst.SetProvenanceVerification(mode, provenance.CosignIdentity{
		IdentityRegexp: cfg.CosignIdentityRegexp,
		OIDCIssuer:     cfg.CosignOIDCIssuer,
	})
	output.Debugf("Provenance verification enabled (mode=%s)", mode)
}
