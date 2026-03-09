package cmd

import (
	"github.com/sid-technologies/scuta/lib/config"
	"github.com/sid-technologies/scuta/lib/output"
	"github.com/sid-technologies/scuta/lib/policy"
)

// loadPolicy tries to load a policy from a remote URL (if configured),
// then falls back to the local policy file. Returns nil on any error.
func loadPolicy(scutaDir string) *policy.Policy {
	cfg, err := config.Load(scutaDir)
	if err == nil && cfg.PolicyURL != "" {
		p, fetchErr := policy.FetchRemote(cfg.PolicyURL)
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
