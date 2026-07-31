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

	// ConfigURL is a remote URL pointing to an org-wide YAML config.
	// When set, the remote config is fetched (with 1h cache) and merged
	// with the local config (local takes priority).
	ConfigURL string `yaml:"config_url,omitempty"`

	// Telemetry enables opt-in anonymous usage tracking.
	// Disabled by default. When enabled, events are recorded locally.
	Telemetry bool `yaml:"telemetry,omitempty"`

	// RequireSignature makes signature verification mandatory.
	// When true, installs fail if no .sig file is found in the release.
	RequireSignature bool `yaml:"require_signature,omitempty"`

	// SignaturePublicKey is a PEM-encoded public key for verifying release signatures.
	// It is also the trust root for signed remote metadata (registry, policy,
	// remote config). For that reason it is only ever honored from local or
	// system config — never from remotely fetched config.
	SignaturePublicKey string `yaml:"signature_public_key,omitempty"`

	// RequireSignedMetadata makes detached-signature verification mandatory
	// for remotely fetched metadata: the registry (registry_url), remote
	// policy (policy_url), and remote org config (config_url). When true,
	// those fetches fail closed if the .sig is missing or invalid.
	RequireSignedMetadata bool `yaml:"require_signed_metadata,omitempty"`

	// AuditLogDestination controls where audit log entries are sent.
	// Supported values: "" (disabled), "stdout", "syslog", or a webhook URL.
	AuditLogDestination string `yaml:"audit_log_destination,omitempty"`

	// DisableDownloadCache turns off the content-addressed download cache
	// (~/.scuta/cache). The cache only ever stores checksum-verified assets,
	// so it is enabled by default.
	DisableDownloadCache bool `yaml:"disable_download_cache,omitempty"`
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
	return []string{"update_interval", "github_token", "registry_url", "github_base_url", "policy_url", "config_url", "telemetry", "require_signature", "signature_public_key", "require_signed_metadata", "audit_log_destination", "disable_download_cache"}
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
	case "telemetry", "require_signature", "require_signed_metadata", "disable_download_cache":
		return "false"
	default:
		return ""
	}
}

// FieldMap returns a map of config key names to their current values.
func (c Config) FieldMap() map[string]string {
	telemetryStr := "false"
	if c.Telemetry {
		telemetryStr = "true"
	}
	requireSigStr := "false"
	if c.RequireSignature {
		requireSigStr = "true"
	}
	requireSignedMetaStr := "false"
	if c.RequireSignedMetadata {
		requireSignedMetaStr = "true"
	}
	disableCacheStr := "false"
	if c.DisableDownloadCache {
		disableCacheStr = "true"
	}
	return map[string]string{
		"update_interval":         c.UpdateInterval,
		"github_token":            c.GithubToken,
		"registry_url":            c.RegistryURL,
		"github_base_url":         c.GithubBaseURL,
		"policy_url":              c.PolicyURL,
		"config_url":              c.ConfigURL,
		"telemetry":               telemetryStr,
		"require_signature":       requireSigStr,
		"signature_public_key":    c.SignaturePublicKey,
		"require_signed_metadata": requireSignedMetaStr,
		"audit_log_destination":   c.AuditLogDestination,
		"disable_download_cache":  disableCacheStr,
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
	case "config_url":
		c.ConfigURL = value
	case "telemetry":
		c.Telemetry = value == "true" || value == "1" || value == "yes"
	case "require_signature":
		c.RequireSignature = value == "true" || value == "1" || value == "yes"
	case "signature_public_key":
		c.SignaturePublicKey = value
	case "require_signed_metadata":
		c.RequireSignedMetadata = value == "true" || value == "1" || value == "yes"
	case "audit_log_destination":
		c.AuditLogDestination = value
	case "disable_download_cache":
		c.DisableDownloadCache = value == "true" || value == "1" || value == "yes"
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
