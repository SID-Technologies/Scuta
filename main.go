package main

import (
	"github.com/sid-technologies/scuta/cmd"
	"github.com/sid-technologies/scuta/lib/config"
	"github.com/sid-technologies/scuta/lib/path"
	"github.com/sid-technologies/scuta/lib/registry"

	_ "embed"
)

//go:embed registry.yaml
var registryData []byte

func main() {
	registry.SetEmbedded(registryData)

	// Set scuta dir for registry caching and load config (best-effort)
	if dir, err := path.ScutaDir(); err == nil {
		registry.SetScutaDir(dir)

		// Merged view (defaults + system + remote org config + local) so an
		// org-managed registry_url from config_url takes effect. The remote
		// layer is signature-verified against the locally resolved trust
		// root and can never supply that trust root itself.
		cfg, err := config.LoadWithMerge(dir)
		if err == nil && cfg.RegistryURL != "" {
			registry.SetRegistryURL(cfg.RegistryURL)
		}

		// Trust root and fail-closed flag come from local + system config
		// only — never from remotely fetched config.
		trust := config.LoadTrusted(dir)
		if trust.SignaturePublicKey != "" || trust.RequireSignedMetadata {
			registry.SetVerification([]byte(trust.SignaturePublicKey), trust.RequireSignedMetadata)
		}
	}

	cmd.Execute()
}
