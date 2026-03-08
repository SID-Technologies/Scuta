// Package config manages ~/.scuta/config.yaml read/write operations.
package config

import (
	"os"
	"path/filepath"
	"time"

	"github.com/sid-technologies/scuta/lib/errors"

	"gopkg.in/yaml.v3"
)

const configFile = "config.yaml"

// Config represents the user's Scuta configuration.
type Config struct {
	// UpdateInterval is how often to check for updates (default: 24h).
	UpdateInterval string `yaml:"update_interval,omitempty"`

	// GithubToken is an optional GitHub token for private repo access.
	GithubToken string `yaml:"github_token,omitempty"`
}

// DefaultConfig returns a Config with default values.
func DefaultConfig() Config {
	return Config{
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
