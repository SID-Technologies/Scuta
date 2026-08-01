package config

import (
	"os"
	"path/filepath"
	"time"

	"github.com/sid-technologies/scuta/lib/errors"
	"github.com/sid-technologies/scuta/lib/fetch"
	"github.com/sid-technologies/scuta/lib/output"
	"github.com/sid-technologies/scuta/lib/provenance"

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
//
// Trust boundary: the signature trust root (signature_public_key) and the
// require_signed_metadata flag are resolved from local + system config BEFORE
// the remote fetch, and the remote config can never supply or replace the
// public key (see mergeRemoteConfig). Otherwise a compromised config host
// could hand out its own key and defeat every downstream verification.
func LoadWithMerge(scutaDir string) (Config, error) {
	// Start with defaults
	cfg := DefaultConfig()

	// Layer 1: system-wide config (lowest priority)
	if sysCfg, err := loadFile(systemConfigPath); err == nil {
		mergeConfig(&cfg, sysCfg)
	}

	// Layer 2: remote org config (if config_url is set — fetch or use cache).
	// The fetch is verified against the locally resolved trust root.
	localCfg, localErr := Load(scutaDir)
	if localErr == nil && localCfg.ConfigURL != "" {
		trust := LoadTrusted(scutaDir)
		if remoteCfg, err := fetchRemoteConfig(scutaDir, localCfg.ConfigURL, trust); err == nil {
			mergeRemoteConfig(&cfg, remoteCfg)
		} else {
			output.Debugf("Remote config not applied: %v", err)
		}
	}

	// Layer 3: local user config (highest priority)
	if localErr == nil {
		mergeConfig(&cfg, localCfg)
	}

	return cfg, nil
}

// LoadTrusted merges defaults, system-wide, and local user config only —
// never remotely fetched config. Use it to resolve security-critical settings
// (the signature trust root and fail-closed flags) that must not be
// influenced by data fetched over the network.
func LoadTrusted(scutaDir string) Config {
	cfg := DefaultConfig()

	if sysCfg, err := loadFile(systemConfigPath); err == nil {
		mergeConfig(&cfg, sysCfg)
	}

	if localCfg, err := Load(scutaDir); err == nil {
		mergeConfig(&cfg, localCfg)
	}

	return cfg
}

// loadFile loads a Config from a specific YAML file path.
func loadFile(filePath string) (Config, error) {
	data, err := os.ReadFile(filePath) //nolint:gosec // fixed system path or scuta dir
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
// When the trust config carries a public key, the payload is verified against
// the detached signature at <url>.sig; with require_signed_metadata the fetch
// fails closed and only a previously verified cache may be used.
func fetchRemoteConfig(scutaDir string, configURL string, trust Config) (Config, error) {
	cachePath := filepath.Join(scutaDir, remoteConfigCache)

	// Check if cache is fresh
	if info, err := os.Stat(cachePath); err == nil {
		if time.Since(info.ModTime()) < remoteConfigCacheTTL {
			if cfg, err := loadFile(cachePath); err == nil {
				return cfg, nil
			}
		}
	}

	cfg, err := FetchRemote(scutaDir, configURL, trust)
	if err != nil {
		// Fall back to the cached version (written only after a successful
		// fetch, so under require_signed_metadata it was verified at write).
		if cached, cacheErr := loadFile(cachePath); cacheErr == nil {
			output.Debugf("Using cached remote config after fetch failure: %v", err)
			return cached, nil
		}
		return Config{}, err
	}

	return cfg, nil
}

// FetchRemote fetches, verifies, and parses an org config from a URL,
// always hitting the network (no cache read). `scuta init --from` uses it
// to bootstrap a machine: the first fetch must go to the source so a bad
// URL, unreachable host, or invalid signature fails loudly instead of
// silently reusing stale state. On success the payload is written to the
// local cache for the regular LoadWithMerge path.
func FetchRemote(scutaDir string, configURL string, trust Config) (Config, error) {
	data, err := fetch.Verified(configURL, fetch.Options{
		PublicKeyPEM:     []byte(trust.SignaturePublicKey),
		RequireSignature: trust.RequireSignedMetadata,
		MaxSize:          maxRemoteConfigSize,
	})
	if err != nil {
		return Config{}, errors.Wrap(err, "fetching remote config from %s", configURL)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, errors.Wrap(err, "parsing remote config")
	}

	// Cache the result
	_ = os.MkdirAll(scutaDir, 0o700)
	_ = os.WriteFile(filepath.Join(scutaDir, remoteConfigCache), data, 0o600)

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
	if src.RequireSignedMetadata {
		dst.RequireSignedMetadata = true
	}
	if src.SignaturePublicKey != "" {
		dst.SignaturePublicKey = src.SignaturePublicKey
	}
	if src.AuditLogDestination != "" {
		dst.AuditLogDestination = src.AuditLogDestination
	}
	if src.DisableDownloadCache {
		dst.DisableDownloadCache = true
	}
	if src.ProvenanceVerify != "" {
		dst.ProvenanceVerify = src.ProvenanceVerify
	}
	if src.CosignIdentityRegexp != "" {
		dst.CosignIdentityRegexp = src.CosignIdentityRegexp
	}
	if src.CosignOIDCIssuer != "" {
		dst.CosignOIDCIssuer = src.CosignOIDCIssuer
	}
}

// mergeRemoteConfig applies a remotely fetched config onto dst. It is
// mergeConfig minus the trust root: a remote config may strengthen security
// settings (flip require flags on) but must never supply or replace the
// signature public key — that would let whoever controls (or intercepts) the
// config host substitute their own key and defeat verification everywhere.
func mergeRemoteConfig(dst *Config, src Config) {
	if src.SignaturePublicKey != "" {
		output.Warning("Ignoring signature_public_key from remote config — the trust root must be set locally")
		src.SignaturePublicKey = ""
	}

	// The cosign identity constraints are trust-root-like: whoever sets
	// them decides which signer counts as valid. Remote config must not.
	if src.CosignIdentityRegexp != "" || src.CosignOIDCIssuer != "" {
		output.Warning("Ignoring cosign identity settings from remote config — signer identity must be set locally")
		src.CosignIdentityRegexp = ""
		src.CosignOIDCIssuer = ""
	}

	// A remote config may strengthen provenance verification but never
	// weaken a locally configured mode (off < auto < require).
	if src.ProvenanceVerify != "" && provenance.Rank(src.ProvenanceVerify) < provenance.Rank(dst.ProvenanceVerify) {
		output.Warning("Ignoring provenance_verify=%q from remote config — weaker than local setting", src.ProvenanceVerify)
		src.ProvenanceVerify = ""
	}

	mergeConfig(dst, src)
}
