// Package updater handles auto-update checks and self-update functionality.
package updater

import (
	"context"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/sid-technologies/scuta/lib/errors"
	"github.com/sid-technologies/scuta/lib/github"
	"github.com/sid-technologies/scuta/lib/output"
	"github.com/sid-technologies/scuta/lib/registry"
	"github.com/sid-technologies/scuta/lib/state"
)

const scutaRepo = "sid-technologies/Scuta"

// UpdateAvailable describes a tool that has a newer version.
type UpdateAvailable struct {
	Name           string
	CurrentVersion string
	LatestVersion  string
	Repo           string
}

// Updater checks for tool and self updates.
type Updater struct {
	github *github.Client
}

// New creates an Updater.
func New(ghClient *github.Client) *Updater {
	return &Updater{
		github: ghClient,
	}
}

// CheckForUpdates checks all installed tools for available updates.
func (u *Updater) CheckForUpdates(ctx context.Context, installed map[string]state.ToolState, tools map[string]registry.Tool) []UpdateAvailable {
	var updates []UpdateAvailable

	for name, ts := range installed {
		tool, ok := tools[name]
		if !ok {
			continue
		}

		release, err := u.github.GetLatestRelease(ctx, tool.Repo)
		if err != nil {
			output.Debugf("Failed to check %s: %v", name, err)
			continue
		}

		latestVersion := github.NormalizeVersion(release.TagName)
		currentVersion := github.NormalizeVersion(ts.Version)

		if CompareVersions(currentVersion, latestVersion) {
			updates = append(updates, UpdateAvailable{
				Name:           name,
				CurrentVersion: currentVersion,
				LatestVersion:  latestVersion,
				Repo:           tool.Repo,
			})
		}
	}

	return updates
}

// NeedsCheck returns true if enough time has elapsed since the last check.
func NeedsCheck(lastCheck time.Time, interval time.Duration) bool {
	return time.Since(lastCheck) > interval
}

// CheckSelfUpdate checks if a newer version of Scuta is available.
func (u *Updater) CheckSelfUpdate(ctx context.Context, currentVersion string) (*UpdateAvailable, error) {
	release, err := u.github.GetLatestRelease(ctx, scutaRepo)
	if err != nil {
		return nil, errors.Wrap(err, "checking for Scuta updates")
	}

	latestVersion := github.NormalizeVersion(release.TagName)
	current := github.NormalizeVersion(currentVersion)

	if !CompareVersions(current, latestVersion) {
		return nil, nil
	}

	return &UpdateAvailable{
		Name:           "scuta",
		CurrentVersion: current,
		LatestVersion:  latestVersion,
		Repo:           scutaRepo,
	}, nil
}

// IsHomebrew returns true if the current binary was installed via Homebrew.
func IsHomebrew() bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}

	exe = strings.ToLower(exe)
	return strings.Contains(exe, "cellar") ||
		strings.Contains(exe, "homebrew") ||
		strings.Contains(exe, "linuxbrew")
}

// CompareVersions returns true if latest is newer than installed.
// Both versions should have the "v" prefix already stripped.
func CompareVersions(installed string, latest string) bool {
	if installed == latest {
		return false
	}

	if installed == "" || installed == "dev" {
		return true
	}

	iParts := parseVersion(installed)
	lParts := parseVersion(latest)

	for i := 0; i < 3; i++ {
		if lParts[i] > iParts[i] {
			return true
		}
		if lParts[i] < iParts[i] {
			return false
		}
	}

	return false
}

// parseVersion splits a version string into [major, minor, patch].
func parseVersion(v string) [3]int {
	v = strings.TrimPrefix(v, "v")
	parts := strings.SplitN(v, ".", 3)

	var result [3]int
	for i := 0; i < 3 && i < len(parts); i++ {
		// Strip any pre-release suffix (e.g., "1-rc1" → "1")
		numStr := strings.SplitN(parts[i], "-", 2)[0]
		n, err := strconv.Atoi(numStr)
		if err == nil {
			result[i] = n
		}
	}

	return result
}
