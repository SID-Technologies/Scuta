// Package registry parses and manages the tool registry manifest.
package registry

import (
	stderrors "errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/sid-technologies/scuta/lib/errors"
	"github.com/sid-technologies/scuta/lib/output"

	"gopkg.in/yaml.v3"
)

const (
	// remoteRegistryURL is the canonical registry location on GitHub.
	remoteRegistryURL = "https://raw.githubusercontent.com/sid-technologies/Scuta/main/registry.yaml"

	// cacheFile is the filename for the cached remote registry.
	cacheFile = "registry.yaml"

	// cacheTTL is how long the cached registry is considered fresh.
	cacheTTL = 1 * time.Hour
)

// embeddedRegistry holds the registry YAML data, set at startup via SetEmbedded.
var embeddedRegistry []byte

// registryScutaDir is set by callers to enable caching. Optional.
var registryScutaDir string

// SetEmbedded sets the embedded registry data. Called from main.go.
func SetEmbedded(data []byte) {
	embeddedRegistry = data
}

// SetScutaDir sets the scuta directory for registry caching.
func SetScutaDir(dir string) {
	registryScutaDir = dir
}

// Tool represents a single tool in the registry.
type Tool struct {
	Description string   `yaml:"description"`
	Repo        string   `yaml:"repo"`
	Homebrew    string   `yaml:"homebrew,omitempty"`
	DependsOn   []string `yaml:"depends_on,omitempty"`
}

// Registry holds the parsed tool manifest.
type Registry struct {
	Tools map[string]Tool `yaml:"tools"`
}

// Load parses the registry, trying remote → cache → embedded in order.
func Load() (*Registry, error) {
	// Try cached remote registry first (if fresh)
	if registryScutaDir != "" {
		if reg, err := loadCache(registryScutaDir); err == nil {
			output.Debugf("Using cached registry")
			return reg, nil
		}
	}

	// Try fetching from remote
	if data, err := fetchRemote(); err == nil {
		output.Debugf("Fetched remote registry")
		reg, parseErr := parse(data)
		if parseErr == nil {
			// Cache the remote data for next time
			if registryScutaDir != "" {
				saveCache(registryScutaDir, data)
			}
			return reg, nil
		}
		output.Debugf("Remote registry parse failed, falling back: %v", parseErr)
	} else {
		output.Debugf("Remote registry fetch failed, falling back: %v", err)
	}

	// Fall back to embedded
	return parse(embeddedRegistry)
}

// LoadEmbedded explicitly loads only the embedded registry (no network).
func LoadEmbedded() (*Registry, error) {
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

// fetchRemote downloads the registry from GitHub.
func fetchRemote() ([]byte, error) {
	client := &http.Client{Timeout: 5 * time.Second}

	resp, err := client.Get(remoteRegistryURL) //nolint:noctx // simple GET with timeout
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("remote registry returned %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return data, nil
}

// loadCache reads the cached registry if it exists and is fresh.
func loadCache(scutaDir string) (*Registry, error) {
	fp := filepath.Join(scutaDir, cacheFile)

	info, err := os.Stat(fp)
	if err != nil {
		return nil, err
	}

	// Check if cache is still fresh
	if time.Since(info.ModTime()) > cacheTTL {
		return nil, stderrors.New("cache expired")
	}

	data, err := os.ReadFile(fp)
	if err != nil {
		return nil, err
	}

	return parse(data)
}

// saveCache writes registry data to the cache file.
func saveCache(scutaDir string, data []byte) {
	fp := filepath.Join(scutaDir, cacheFile)
	_ = os.MkdirAll(scutaDir, 0o700)
	_ = os.WriteFile(fp, data, 0o600)
}
