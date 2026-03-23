package config

import (
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/sid-technologies/scuta/lib/errors"
)

// ValidateValue validates a config value for the given key.
func ValidateValue(key, value string) error {
	switch key {
	case "update_interval":
		if _, err := time.ParseDuration(value); err != nil {
			return errors.New("invalid duration for update_interval: %q (examples: 12h, 30m, 24h)", value)
		}
	case "registry_url":
		if value == "local" {
			break
		}
		if err := validateURL(key, value); err != nil {
			return err
		}
	case "config_url", "github_base_url", "policy_url":
		if value == "" {
			break
		}
		if err := validateURL(key, value); err != nil {
			return err
		}
	case "telemetry", "require_signature":
		valid := map[string]bool{"true": true, "false": true, "1": true, "0": true, "yes": true, "no": true}
		if !valid[strings.ToLower(value)] {
			return errors.New("invalid value for %s: %q (use true/false)", key, value)
		}
	case "audit_log_destination":
		if value == "" || value == "stdout" || value == "syslog" {
			break
		}
		// Must be a valid webhook URL
		if err := validateURL(key, value); err != nil {
			return errors.New("invalid value for %s: %q (use \"\", \"stdout\", \"syslog\", or a webhook URL)", key, value)
		}
	default:
		// No validation for other keys (e.g. github_token)
	}
	return nil
}

// validateURL validates that a URL is safe: HTTPS-only, no loopback, no private IPs.
func validateURL(key, value string) error {
	u, err := url.Parse(value)
	if err != nil {
		return errors.New("invalid URL for %s: %q", key, value)
	}

	if u.Scheme != "https" {
		return errors.New("invalid URL for %s: %q (only https is allowed)", key, value)
	}

	host := u.Hostname()
	if host == "" {
		return errors.New("invalid URL for %s: %q (missing host)", key, value)
	}

	// Check for loopback and private IPs
	if ip := net.ParseIP(host); ip != nil {
		if ip.IsLoopback() {
			return errors.New("invalid URL for %s: %q (loopback addresses are not allowed)", key, value)
		}
		if ip.IsPrivate() || ip.IsLinkLocalUnicast() {
			return errors.New("invalid URL for %s: %q (private/link-local addresses are not allowed)", key, value)
		}
	}

	// Also catch "localhost" by name
	if strings.EqualFold(host, "localhost") {
		return errors.New("invalid URL for %s: %q (localhost is not allowed)", key, value)
	}

	return nil
}

// MaskValue masks sensitive config values for display.
func MaskValue(key, value string) string {
	if key == "github_token" && value != "" {
		return "****"
	}
	return value
}
