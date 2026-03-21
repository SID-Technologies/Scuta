package config

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/sid-technologies/scuta/lib/errors"

	"gopkg.in/yaml.v3"
)

const (
	remoteConfigCache    = "remote_config.yaml"
	remoteConfigCacheTTL = 1 * time.Hour
	systemConfigPath     = "/etc/scuta/config.yaml"
	maxRemoteConfigSize  = 1 * 1024 * 1024 // 1 MB
)

// LoadWithMerge loads the effective config by merging multiple sources.
// Priority (highest to lowest): local user config > remote org config > system-wide config > defaults.
func LoadWithMerge(scutaDir string) (Config, error) {
	// Start with defaults
	cfg := DefaultConfig()

	// Layer 1: system-wide config (lowest priority)
	if sysCfg, err := loadFile(systemConfigPath); err == nil {
		mergeConfig(&cfg, sysCfg)
	}

	// Layer 2: remote org config (if config_url is set — fetch or use cache)
	// We need to peek at the local config first to get the config_url
	localCfg, localErr := Load(scutaDir)
	if localErr == nil && localCfg.ConfigURL != "" {
		if remoteCfg, err := fetchRemoteConfig(scutaDir, localCfg.ConfigURL); err == nil {
			mergeConfig(&cfg, remoteCfg)
		}
	}

	// Layer 3: local user config (highest priority)
	if localErr == nil {
		mergeConfig(&cfg, localCfg)
	}

	return cfg, nil
}

// loadFile loads a Config from a specific YAML file path.
func loadFile(filePath string) (Config, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return Config{}, errors.Wrap(err, "reading config file %s", filePath)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, errors.Wrap(err, "parsing config file %s", filePath)
	}

	return cfg, nil
}

// fetchRemoteConfig fetches a remote config from a URL, with local caching.
// Uses the cached version if it's fresh enough (< remoteConfigCacheTTL).
func fetchRemoteConfig(scutaDir string, configURL string) (Config, error) {
	cachePath := filepath.Join(scutaDir, remoteConfigCache)

	// Check if cache is fresh
	if info, err := os.Stat(cachePath); err == nil {
		if time.Since(info.ModTime()) < remoteConfigCacheTTL {
			if cfg, err := loadFile(cachePath); err == nil {
				return cfg, nil
			}
		}
	}

	// Fetch from remote
	resp, err := http.Get(configURL) //nolint:gosec,noctx // config URL is user-configured
	if err != nil {
		// Fall back to cached version on failure
		if cfg, cacheErr := loadFile(cachePath); cacheErr == nil {
			return cfg, nil
		}
		return Config{}, errors.Wrap(err, "fetching remote config from %s", configURL)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Fall back to cached version
		if cfg, cacheErr := loadFile(cachePath); cacheErr == nil {
			return cfg, nil
		}
		return Config{}, errors.New("remote config returned %d", resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxRemoteConfigSize))
	if err != nil {
		return Config{}, errors.Wrap(err, "reading remote config")
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, errors.Wrap(err, "parsing remote config")
	}

	// Cache the result
	_ = os.MkdirAll(scutaDir, 0o700)
	_ = os.WriteFile(cachePath, data, 0o600)

	return cfg, nil
}

// mergeConfig applies non-zero values from src onto dst.
// This implements "src overrides dst" semantics.
func mergeConfig(dst *Config, src Config) {
	if src.UpdateInterval != "" {
		dst.UpdateInterval = src.UpdateInterval
	}
	if src.GithubToken != "" {
		dst.GithubToken = src.GithubToken
	}
	if src.RegistryURL != "" {
		dst.RegistryURL = src.RegistryURL
	}
	if src.GithubBaseURL != "" {
		dst.GithubBaseURL = src.GithubBaseURL
	}
	if src.PolicyURL != "" {
		dst.PolicyURL = src.PolicyURL
	}
	if src.ConfigURL != "" {
		dst.ConfigURL = src.ConfigURL
	}
	if src.Telemetry {
		dst.Telemetry = true
	}
	if src.RequireSignature {
		dst.RequireSignature = true
	}
	if src.SignaturePublicKey != "" {
		dst.SignaturePublicKey = src.SignaturePublicKey
	}
	if src.AuditLogDestination != "" {
		dst.AuditLogDestination = src.AuditLogDestination
	}
}
