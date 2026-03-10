package config

import (
	"net/url"
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
		u, err := url.Parse(value)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return errors.New("invalid URL for registry_url: %q (must include scheme and host, or \"local\")", value)
		}
	default:
		// No validation for other keys (e.g. github_token)
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
