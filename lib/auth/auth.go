// Package auth provides GitHub token resolution for accessing private repos.
// Token resolution order: SCUTA_GITHUB_TOKEN env → config file → gh auth token → none.
package auth

import (
	"os"
	"os/exec"
	"strings"

	"github.com/sid-technologies/scuta/lib/config"
	"github.com/sid-technologies/scuta/lib/output"
)

// ResolveToken attempts to find a GitHub token using the resolution chain:
// 1. SCUTA_GITHUB_TOKEN environment variable
// 2. Config file github_token field
// 3. gh auth token (GitHub CLI)
// Returns empty string if no token is found.
func ResolveToken() string {
	return ResolveTokenWithConfig("")
}

// ResolveTokenWithConfig attempts to find a GitHub token, including config file lookup.
func ResolveTokenWithConfig(scutaDir string) string {
	// 1. Environment variable
	if token := os.Getenv("SCUTA_GITHUB_TOKEN"); token != "" {
		output.Debugf("Using GitHub token from SCUTA_GITHUB_TOKEN env var")
		return token
	}

	// 2. Config file
	if scutaDir != "" {
		cfg, err := config.Load(scutaDir)
		if err == nil && cfg.GithubToken != "" {
			output.Debugf("Using GitHub token from config file")
			return cfg.GithubToken
		}
	}

	// 3. gh CLI
	token, err := ghAuthToken()
	if err == nil && token != "" {
		output.Debugf("Using GitHub token from gh CLI")
		return token
	}

	output.Debugf("No GitHub token found — private repos won't be accessible")
	return ""
}

// ghAuthToken attempts to get a token from the GitHub CLI.
func ghAuthToken() (string, error) {
	cmd := exec.Command("gh", "auth", "token")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// HasToken returns true if a GitHub token is available.
func HasToken() bool {
	return ResolveToken() != ""
}
