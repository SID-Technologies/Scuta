package cmd

import (
	"github.com/sid-technologies/scuta/lib/config"
	"github.com/sid-technologies/scuta/lib/output"
	"github.com/sid-technologies/scuta/lib/policy"
)

// loadPolicy tries to load a policy from a remote URL (if configured),
// then falls back to the local policy file. Returns nil on any error.
func loadPolicy(scutaDir string) *policy.Policy {
	// LoadTrusted resolves the trust root from local + system config only,
	// so a remotely fetched config can never influence policy verification.
	cfg := config.LoadTrusted(scutaDir)
	if cfg.PolicyURL != "" {
		p, fetchErr := policy.FetchRemote(cfg.PolicyURL, []byte(cfg.SignaturePublicKey), cfg.RequireSignedMetadata)
		if fetchErr == nil {
			return p
		}
		output.Debugf("Failed to fetch remote policy: %v", fetchErr)
	}

	p, err := policy.Load(scutaDir)
	if err != nil {
		output.Debugf("Failed to load local policy: %v", err)
		return nil
	}

	return p
}
