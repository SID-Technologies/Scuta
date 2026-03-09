package cmd

import (
	"github.com/sid-technologies/scuta/lib/config"
	"github.com/sid-technologies/scuta/lib/github"
	"github.com/sid-technologies/scuta/lib/output"
)

// newGitHubClient creates a GitHub client with token and optional base URL from config.
func newGitHubClient(token string, scutaDir string) *github.Client {
	client := github.NewClient(token)

	cfg, err := config.Load(scutaDir)
	if err != nil {
		return client
	}

	if cfg.GithubBaseURL != "" {
		client.SetBaseURL(cfg.GithubBaseURL)
		output.Debugf("Using GitHub base URL: %s", cfg.GithubBaseURL)
	}

	return client
}
