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
	// defaultRemoteURL is the canonical registry location on GitHub.
	defaultRemoteURL = "https://raw.githubusercontent.com/sid-technologies/Scuta/main/registry.yaml"

	// cacheFile is the filename for the cached remote registry.
	cacheFile = "registry.yaml"

	// localFile is the filename for user-defined local tools.
	localFile = "local.yaml"

	// cacheTTL is how long the cached registry is considered fresh.
	cacheTTL = 1 * time.Hour
)

// Source identifies where a tool definition came from.
const (
	SourceLocal    = "local"
	SourceRemote   = "remote"
	SourceEmbedded = "embedded"
)

// embeddedRegistry holds the registry YAML data, set at startup via SetEmbedded.
var embeddedRegistry []byte

// registryScutaDir is set by callers to enable caching. Optional.
var registryScutaDir string

// customRemoteURL overrides the default remote registry URL when set.
var customRemoteURL string

// SetEmbedded sets the embedded registry data. Called from main.go.
func SetEmbedded(data []byte) {
	embeddedRegistry = data
}

// SetScutaDir sets the scuta directory for registry caching.
func SetScutaDir(dir string) {
	registryScutaDir = dir
}

// SetRegistryURL overrides the default remote registry URL.
// If url is empty, the default GitHub URL is used.
// Set to "local" to disable remote fetching entirely.
func SetRegistryURL(url string) {
	customRemoteURL = url
}

// isLocalOnly returns true when the user has opted out of remote registries.
func isLocalOnly() bool {
	return customRemoteURL == "local"
}

// remoteURL returns the effective remote registry URL.
func remoteURL() string {
	if customRemoteURL != "" {
		return customRemoteURL
	}
	return defaultRemoteURL
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
	Tools   map[string]Tool   `yaml:"tools"`
	Sources map[string]string `yaml:"-"`
}

// Load parses the registry, trying cache → remote → embedded,
// then merges the local registry on top (local wins on conflicts).
func Load() (*Registry, error) {
	reg, source, err := loadMain()
	if err != nil {
		return nil, err
	}

	// Tag all tools with their source
	reg.Sources = make(map[string]string, len(reg.Tools))
	for name := range reg.Tools {
		reg.Sources[name] = source
	}

	// Merge local registry on top (highest priority)
	if registryScutaDir != "" {
		local, localErr := loadLocal(registryScutaDir)
		if localErr != nil {
			output.Debugf("Failed to load local registry: %v", localErr)
		} else if len(local.Tools) > 0 {
			Merge(reg, local)
			output.Debugf("Merged %d local tool(s)", len(local.Tools))
		}
	}

	return reg, nil
}

// loadMain loads the main registry: cache → remote → embedded.
// When registry_url is "local", remote fetching is skipped entirely.
func loadMain() (*Registry, string, error) {
	if isLocalOnly() {
		output.Debugf("Local-only mode — skipping remote registry")
		reg, err := parse(embeddedRegistry)
		if err != nil {
			return nil, "", err
		}
		return reg, SourceEmbedded, nil
	}

	// Try cached remote registry first (if fresh)
	if registryScutaDir != "" {
		if reg, err := loadCache(registryScutaDir); err == nil {
			output.Debugf("Using cached registry")
			return reg, SourceRemote, nil
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
			return reg, SourceRemote, nil
		}
		output.Debugf("Remote registry parse failed, falling back: %v", parseErr)
	} else {
		output.Debugf("Remote registry fetch failed, falling back: %v", err)
	}

	// Fall back to embedded
	reg, err := parse(embeddedRegistry)
	if err != nil {
		return nil, "", err
	}
	return reg, SourceEmbedded, nil
}

// LoadEmbedded explicitly loads only the embedded registry (no network).
func LoadEmbedded() (*Registry, error) {
	return parse(embeddedRegistry)
}

// LoadLocal loads only the local registry from ~/.scuta/local.yaml.
func LoadLocal(scutaDir string) (*Registry, error) {
	return loadLocal(scutaDir)
}

// SaveLocal writes the local registry to ~/.scuta/local.yaml.
func SaveLocal(scutaDir string, reg *Registry) error {
	if err := os.MkdirAll(scutaDir, 0o700); err != nil {
		return errors.Wrap(err, "creating scuta directory")
	}

	data, err := yaml.Marshal(reg)
	if err != nil {
		return errors.Wrap(err, "marshaling local registry")
	}

	fp := filepath.Join(scutaDir, localFile)
	if err := os.WriteFile(fp, data, 0o600); err != nil {
		return errors.Wrap(err, "writing local registry")
	}

	return nil
}

// Merge copies all tools from overlay into base. Overlay wins on name conflicts.
// Sources are updated to reflect the overlay origin.
func Merge(base, overlay *Registry) {
	if base.Sources == nil {
		base.Sources = make(map[string]string)
	}

	for name, tool := range overlay.Tools {
		base.Tools[name] = tool

		source := SourceLocal
		if overlay.Sources != nil {
			if s, ok := overlay.Sources[name]; ok {
				source = s
			}
		}
		base.Sources[name] = source
	}
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

// Source returns the source of a tool (local, remote, embedded).
// Returns empty string if the tool is not found or sources aren't tracked.
func (r *Registry) Source(name string) string {
	if r.Sources == nil {
		return ""
	}
	return r.Sources[name]
}

// fetchRemote downloads the registry from the remote URL.
func fetchRemote() ([]byte, error) {
	client := &http.Client{Timeout: 5 * time.Second}

	resp, err := client.Get(remoteURL()) //nolint:noctx // simple GET with timeout
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

// loadLocal reads the local registry from ~/.scuta/local.yaml.
// Returns an empty registry if the file doesn't exist.
func loadLocal(scutaDir string) (*Registry, error) {
	fp := filepath.Join(scutaDir, localFile)

	data, err := os.ReadFile(fp)
	if err != nil {
		if os.IsNotExist(err) {
			return &Registry{Tools: make(map[string]Tool)}, nil
		}
		return nil, errors.Wrap(err, "reading local registry")
	}

	reg, err := parse(data)
	if err != nil {
		return nil, errors.Wrap(err, "parsing local registry")
	}

	// Tag all local tools
	reg.Sources = make(map[string]string, len(reg.Tools))
	for name := range reg.Tools {
		reg.Sources[name] = SourceLocal
	}

	return reg, nil
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
