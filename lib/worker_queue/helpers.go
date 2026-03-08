package workerqueue

import (
	"math"
	"math/rand"
	"strings"
)

// ExponentialBackoffWithJitter calculates exponential backoff with jitter.
// Returns delay in seconds.
func ExponentialBackoffWithJitter(attempt int, baseDelay float64, maxDelay float64) float64 {
	delay := math.Min(maxDelay, baseDelay*math.Pow(2, float64(attempt)))

	// Add jitter (random variation between 0.5x and 1.5x of calculated delay)
	//nolint:gosec // It's a random number, not a secret
	jitteredDelay := delay * (0.5 + rand.Float64())

	return jitteredDelay
}

// sensitiveEnvSuffixes contains key suffixes that indicate secrets.
var sensitiveEnvSuffixes = []string{
	"_TOKEN",
	"_SECRET",
	"_KEY",
	"_PASSWORD",
	"_CREDENTIAL",
	"_API_KEY",
}

// ShouldRedact returns true if the env var key matches a sensitive pattern.
func ShouldRedact(key string) bool {
	upper := strings.ToUpper(key)
	for _, suffix := range sensitiveEnvSuffixes {
		if strings.HasSuffix(upper, suffix) {
			return true
		}
	}
	return false
}

// RedactEnvVars returns a copy of the env vars map with sensitive values replaced.
func RedactEnvVars(envVars map[string]string) map[string]string {
	redacted := make(map[string]string, len(envVars))
	for k, v := range envVars {
		if ShouldRedact(k) {
			redacted[k] = "[REDACTED]"
		} else {
			redacted[k] = v
		}
	}
	return redacted
}
