// Package config manages ~/.scuta/config.yaml read/write operations.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/sid-technologies/scuta/lib/errors"

	"gopkg.in/yaml.v3"
)

const configFile = "config.yaml"

// CurrentConfigVersion is the current config file format version.
const CurrentConfigVersion = 1

// Config represents the user's Scuta configuration.
type Config struct {
	// Version tracks the config file format version for migrations.
	Version int `yaml:"version,omitempty"`

	// UpdateInterval is how often to check for updates (default: 24h).
	UpdateInterval string `yaml:"update_interval,omitempty"`

	// GithubToken is an optional GitHub token for private repo access.
	GithubToken string `yaml:"github_token,omitempty"`

	// RegistryURL overrides the default remote registry URL.
	RegistryURL string `yaml:"registry_url,omitempty"`

	// GithubBaseURL overrides the GitHub API base URL for GitHub Enterprise.
	// Example: https://github.example.com/api/v3
	GithubBaseURL string `yaml:"github_base_url,omitempty"`

	// PolicyURL is a remote URL to fetch policy.yaml from.
	PolicyURL string `yaml:"policy_url,omitempty"`
}

// DefaultConfig returns a Config with default values.
func DefaultConfig() Config {
	return Config{
		Version:        CurrentConfigVersion,
		UpdateInterval: "24h",
	}
}

// Load reads the config from ~/.scuta/config.yaml.
// Returns default config if the file doesn't exist.
func Load(scutaDir string) (Config, error) {
	fp := filepath.Join(scutaDir, configFile)

	data, err := os.ReadFile(fp)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultConfig(), nil
		}
		return Config{}, errors.Wrap(err, "reading config file")
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, errors.Wrap(err, "parsing config file")
	}

	// Auto-migrate pre-versioned config files
	if cfg.Version == 0 {
		cfg.Version = CurrentConfigVersion
		_ = Save(scutaDir, cfg)
	}

	return cfg, nil
}

// Save writes the config to ~/.scuta/config.yaml.
func Save(scutaDir string, cfg Config) error {
	if err := os.MkdirAll(scutaDir, 0o700); err != nil {
		return errors.Wrap(err, "creating scuta directory")
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return errors.Wrap(err, "marshaling config")
	}

	fp := filepath.Join(scutaDir, configFile)
	if err := os.WriteFile(fp, data, 0o600); err != nil {
		return errors.Wrap(err, "writing config file")
	}

	return nil
}

// UpdateIntervalDuration parses the update interval as a time.Duration.
func (c Config) UpdateIntervalDuration() time.Duration {
	d, err := time.ParseDuration(c.UpdateInterval)
	if err != nil {
		return 24 * time.Hour
	}
	return d
}

// ValidKeys returns the list of valid configuration keys.
func ValidKeys() []string {
	return []string{"update_interval", "github_token", "registry_url", "github_base_url", "policy_url"}
}

// DefaultValue returns the default value for a given config key.
func DefaultValue(key string) string {
	defaults := DefaultConfig()
	switch key {
	case "update_interval":
		return defaults.UpdateInterval
	case "github_token":
		return defaults.GithubToken
	case "registry_url":
		return defaults.RegistryURL
	case "github_base_url":
		return defaults.GithubBaseURL
	case "policy_url":
		return defaults.PolicyURL
	default:
		return ""
	}
}

// FieldMap returns a map of config key names to their current values.
func (c Config) FieldMap() map[string]string {
	return map[string]string{
		"update_interval": c.UpdateInterval,
		"github_token":    c.GithubToken,
		"registry_url":    c.RegistryURL,
		"github_base_url": c.GithubBaseURL,
		"policy_url":      c.PolicyURL,
	}
}

// SetField sets a config field by its YAML key name.
// Returns an error for unknown keys.
func (c *Config) SetField(key, value string) error {
	switch key {
	case "update_interval":
		c.UpdateInterval = value
	case "github_token":
		c.GithubToken = value
	case "registry_url":
		c.RegistryURL = value
	case "github_base_url":
		c.GithubBaseURL = value
	case "policy_url":
		c.PolicyURL = value
	default:
		return fmt.Errorf("unknown config key: %s", key)
	}
	return nil
}

// ResetField resets a config field to its default value.
// Returns an error for unknown keys.
func (c *Config) ResetField(key string) error {
	return c.SetField(key, DefaultValue(key))
}
