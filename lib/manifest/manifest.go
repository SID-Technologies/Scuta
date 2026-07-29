// Package manifest implements Scuta's declarative desired-state file. A manifest
// lists the tools (and optionally pinned versions/repos) an organization or
// machine should have, and `scuta sync` reconciles the machine to match it.
package manifest

import (
	"os"
	"sort"
	"strings"

	"github.com/sid-technologies/scuta/lib/errors"

	"gopkg.in/yaml.v3"
)

// DefaultFiles are the manifest filenames looked up (in order) when no explicit
// path is given.
var DefaultFiles = []string{"scuta.lock.yaml", "scuta.lock.yml", "scuta.yaml", "scuta.yml"}

// VersionLatest indicates "any version — just ensure the tool is installed".
const VersionLatest = "latest"

// Entry is a single tool requirement. Version may be a pinned semver or
// "latest"/empty. Repo is optional and only needed for tools not in the
// registry (e.g. arbitrary public GitHub repos).
type Entry struct {
	Version string `yaml:"version,omitempty"`
	Repo    string `yaml:"repo,omitempty"`
	Bin     string `yaml:"bin,omitempty"` // installed binary name if it differs from the tool key
}

// UnmarshalYAML accepts either a shorthand scalar (a version string) or a full
// mapping. This lets `pilum: "0.7.5"` and the expanded form both parse.
func (e *Entry) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		e.Version = node.Value
		return nil
	}

	// Alias to avoid infinite recursion into this UnmarshalYAML.
	type entryAlias Entry
	var alias entryAlias
	if err := node.Decode(&alias); err != nil {
		return errors.Wrap(err, "parsing manifest entry")
	}
	*e = Entry(alias)
	return nil
}

// Manifest is the parsed desired-state file.
type Manifest struct {
	Tools map[string]Entry `yaml:"tools"`
}

// Load reads and validates a manifest from the given path.
func Load(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.Wrap(err, "reading manifest %s", path)
	}

	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, errors.Wrap(err, "parsing manifest %s", path)
	}

	if err := m.Validate(); err != nil {
		return nil, err
	}
	return &m, nil
}

// FindDefault returns the first existing default manifest file in dir, or an
// empty string if none is present.
func FindDefault(dir string) string {
	for _, name := range DefaultFiles {
		path := name
		if dir != "" {
			path = dir + string(os.PathSeparator) + name
		}
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path
		}
	}
	return ""
}

// Validate checks the manifest is well-formed.
func (m *Manifest) Validate() error {
	if len(m.Tools) == 0 {
		return errors.New("manifest has no tools listed")
	}
	for name := range m.Tools {
		if strings.TrimSpace(name) == "" {
			return errors.New("manifest contains an empty tool name")
		}
	}
	return nil
}

// Names returns the manifest tool names, sorted.
func (m *Manifest) Names() []string {
	names := make([]string, 0, len(m.Tools))
	for name := range m.Tools {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// NormalizeVersion strips a leading "v" and treats empty/"latest" as latest.
// Returns ("", true) when the entry means "latest".
func NormalizeVersion(v string) (normalized string, isLatest bool) {
	v = strings.TrimSpace(v)
	if v == "" || strings.EqualFold(v, VersionLatest) {
		return "", true
	}
	return strings.TrimPrefix(v, "v"), false
}
