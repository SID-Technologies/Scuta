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

		cfg, err := config.Load(dir)
		if err == nil && cfg.RegistryURL != "" {
			registry.SetRegistryURL(cfg.RegistryURL)
		}
	}

	cmd.Execute()
}
