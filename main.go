package main

import (
	"github.com/sid-technologies/scuta/cmd"
	"github.com/sid-technologies/scuta/lib/registry"

	_ "embed"
)

//go:embed registry.yaml
var registryData []byte

func main() {
	registry.SetEmbedded(registryData)
	cmd.Execute()
}
