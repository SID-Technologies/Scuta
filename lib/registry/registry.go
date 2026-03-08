// Package registry parses and manages the tool registry manifest.
package registry

import (
	"github.com/sid-technologies/scuta/lib/errors"

	"gopkg.in/yaml.v3"
)

// embeddedRegistry holds the registry YAML data, set at startup via SetEmbedded.
var embeddedRegistry []byte

// SetEmbedded sets the embedded registry data. Called from main.go.
func SetEmbedded(data []byte) {
	embeddedRegistry = data
}

// Tool represents a single tool in the registry.
type Tool struct {
	Description string `yaml:"description"`
	Repo        string `yaml:"repo"`
	Homebrew    string `yaml:"homebrew,omitempty"`
}

// Registry holds the parsed tool manifest.
type Registry struct {
	Tools map[string]Tool `yaml:"tools"`
}

// Load parses the embedded registry manifest.
func Load() (*Registry, error) {
	return parse(embeddedRegistry)
}

// parse unmarshals registry YAML data.
func parse(data []byte) (*Registry, error) {
	var reg Registry
	if err := yaml.Unmarshal(data, &reg); err != nil {
		return nil, errors.Wrap(err, "parsing registry")
	}

	if reg.Tools == nil {
		reg.Tools = make(map[string]Tool)
	}

	return &reg, nil
}

// Get returns a tool by name, or false if not found.
func (r *Registry) Get(name string) (Tool, bool) {
	tool, ok := r.Tools[name]
	return tool, ok
}

// Names returns all tool names in the registry.
func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.Tools))
	for name := range r.Tools {
		names = append(names, name)
	}
	return names
}
