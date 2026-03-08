package main

import (
	_ "embed"

	"github.com/sid-technologies/scuta/cmd"
	"github.com/sid-technologies/scuta/lib/registry"
)

//go:embed registry.yaml
var registryData []byte

func main() {
	registry.SetEmbedded(registryData)
	cmd.Execute()
}
