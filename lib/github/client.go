// Package github provides a client for the GitHub Releases API.
package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"

	"github.com/sid-technologies/scuta/lib/errors"
	"github.com/sid-technologies/scuta/lib/output"
)

// Client provides access to GitHub Releases for downloading tool binaries.
type Client struct {
	token      string
	baseURL    string
	httpClient *http.Client
}

// NewClient creates a GitHub API client with an optional auth token.
// The client respects HTTP_PROXY, HTTPS_PROXY, and NO_PROXY environment variables.
func NewClient(token string) *Client {
	return &Client{
		token:   token,
		baseURL: "https://api.github.com",
		httpClient: &http.Client{
			Transport: &http.Transport{
				Proxy: http.ProxyFromEnvironment,
			},
		},
	}
}

// SetBaseURL overrides the GitHub API base URL for GitHub Enterprise support.
// Example: https://github.example.com/api/v3
func (c *Client) SetBaseURL(url string) {
	c.baseURL = strings.TrimRight(url, "/")
}

// Release represents a GitHub release.
type Release struct {
	TagName string  `json:"tag_name"`
	Name    string  `json:"name"`
	Assets  []Asset `json:"assets"`
}

// Asset represents a downloadable file attached to a release.
type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
	ContentType        string `json:"content_type"`
}

// GetLatestRelease fetches the latest release for a given repo (owner/repo format).
func (c *Client) GetLatestRelease(ctx context.Context, repo string) (*Release, error) {
	url := fmt.Sprintf("%s/repos/%s/releases/latest", c.baseURL, repo)
	output.Debugf("Fetching latest release: %s", url)

	return c.fetchRelease(ctx, url)
}

// GetRelease fetches a specific release by tag for a given repo.
func (c *Client) GetRelease(ctx context.Context, repo string, tag string) (*Release, error) {
	if !strings.HasPrefix(tag, "v") {
		tag = "v" + tag
	}
	url := fmt.Sprintf("%s/repos/%s/releases/tags/%s", c.baseURL, repo, tag)
	output.Debugf("Fetching release %s: %s", tag, url)

	return c.fetchRelease(ctx, url)
}

// fetchRelease performs the HTTP request and parses a Release from the response.
func (c *Client) fetchRelease(ctx context.Context, url string) (*Release, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, errors.Wrap(err, "creating request")
	}

	c.addHeaders(req)

	resp, err := doWithRetry(c.httpClient, req, defaultMaxAttempts)
	if err != nil {
		return nil, errors.Wrap(err, "fetching release")
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, errors.New("release not found at %s", url)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("GitHub API returned %d for %s", resp.StatusCode, url)
	}

	var release Release
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, errors.Wrap(err, "parsing release JSON")
	}

	return &release, nil
}

// DownloadChecksums downloads and parses the checksums file from a release.
// It looks for assets named "checksums.txt" or "SHA256SUMS".
// Returns nil, nil if no checksums asset is found (not an error — best-effort).
func (c *Client) DownloadChecksums(ctx context.Context, release *Release) (map[string]string, error) {
	var checksumAsset *Asset
	for i := range release.Assets {
		name := strings.ToLower(release.Assets[i].Name)
		if name == "checksums.txt" || name == "sha256sums" || name == "sha256sums.txt" {
			checksumAsset = &release.Assets[i]
			break
		}
	}

	if checksumAsset == nil {
		return nil, nil
	}

	output.Debugf("Downloading checksums: %s", checksumAsset.BrowserDownloadURL)

	req, err := http.NewRequestWithContext(ctx, "GET", checksumAsset.BrowserDownloadURL, nil)
	if err != nil {
		return nil, errors.Wrap(err, "creating checksums request")
	}

	c.addHeaders(req)
	req.Header.Set("Accept", "application/octet-stream")

	resp, err := doWithRetry(c.httpClient, req, defaultMaxAttempts)
	if err != nil {
		return nil, errors.Wrap(err, "downloading checksums")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("checksums download failed with status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "reading checksums")
	}

	// Parse the checksums file — import the parser from installer package
	// to avoid circular dependency, we inline the parsing here.
	result := make(map[string]string)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, " ", 2)
		if len(parts) != 2 {
			continue
		}
		hash := strings.TrimSpace(parts[0])
		filename := strings.TrimSpace(parts[1])
		filename = strings.TrimLeft(filename, " *")
		if hash != "" && filename != "" {
			result[filename] = strings.ToLower(hash)
		}
	}

	output.Debugf("Parsed %d checksums", len(result))
	return result, nil
}

// DownloadAsset downloads a release asset to the given destination path.
func (c *Client) DownloadAsset(ctx context.Context, url string, dest string) error {
	output.Debugf("Downloading asset: %s → %s", url, dest)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return errors.Wrap(err, "creating download request")
	}

	c.addHeaders(req)
	req.Header.Set("Accept", "application/octet-stream")

	resp, err := doWithRetry(c.httpClient, req, defaultMaxAttempts)
	if err != nil {
		return errors.Wrap(err, "downloading asset")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return errors.New("download failed with status %d", resp.StatusCode)
	}

	f, err := os.Create(dest)
	if err != nil {
		return errors.Wrap(err, "creating destination file")
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		return errors.Wrap(err, "writing downloaded data")
	}

	return nil
}

// addHeaders adds common headers including auth if a token is set.
func (c *Client) addHeaders(req *http.Request) {
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
}

// FindAsset finds the best matching asset for the given OS and architecture.
// It matches GoReleaser naming conventions: {name}_{os}_{arch}.tar.gz
func FindAsset(assets []Asset, goos string, goarch string) (*Asset, error) {
	if len(assets) == 0 {
		return nil, errors.New("release has no assets")
	}

	normalizedOS := normalizeOS(goos)
	normalizedArch := normalizeArch(goarch)

	// Build patterns to try, from most specific to least
	patterns := buildPatterns(normalizedOS, normalizedArch)

	for _, pattern := range patterns {
		for i := range assets {
			name := strings.ToLower(assets[i].Name)
			if matchesPattern(name, pattern) {
				return &assets[i], nil
			}
		}
	}

	// No match found — list available assets in error
	var available []string
	for _, a := range assets {
		available = append(available, a.Name)
	}
	return nil, errors.New(
		"no asset found for %s/%s. Available: %s",
		goos, goarch, strings.Join(available, ", "),
	)
}

// FindAssetAuto finds the best matching asset for the current OS and architecture.
func FindAssetAuto(assets []Asset) (*Asset, error) {
	return FindAsset(assets, runtime.GOOS, runtime.GOARCH)
}

// buildPatterns returns filename patterns to match, from most specific to least.
func buildPatterns(os, arch string) []matchPattern {
	extensions := []string{".tar.gz", ".zip"}

	var patterns []matchPattern
	for _, ext := range extensions {
		// {name}_{os}_{arch}.tar.gz — standard GoReleaser
		patterns = append(patterns, matchPattern{os: os, arch: arch, ext: ext, separator: "_"})
		// {name}-{os}-{arch}.tar.gz — dash separator
		patterns = append(patterns, matchPattern{os: os, arch: arch, ext: ext, separator: "-"})
	}

	return patterns
}

type matchPattern struct {
	os        string
	arch      string
	ext       string
	separator string
}

// matchesPattern checks if a filename matches the given OS/arch/ext pattern.
func matchesPattern(filename string, p matchPattern) bool {
	if !strings.HasSuffix(filename, p.ext) {
		return false
	}

	// Check that the filename contains both os and arch segments
	return strings.Contains(filename, p.separator+p.os+p.separator+p.arch) ||
		strings.Contains(filename, p.separator+p.os+p.separator) && strings.Contains(filename, p.separator+p.arch+".")
}

// normalizeOS normalizes OS names to match GoReleaser conventions.
func normalizeOS(goos string) string {
	switch strings.ToLower(goos) {
	case "darwin":
		return "darwin"
	case "linux":
		return "linux"
	case "windows":
		return "windows"
	default:
		return strings.ToLower(goos)
	}
}

// normalizeArch normalizes architecture names to match GoReleaser conventions.
func normalizeArch(goarch string) string {
	switch strings.ToLower(goarch) {
	case "amd64", "x86_64":
		return "amd64"
	case "arm64", "aarch64":
		return "arm64"
	case "386", "i386", "i686":
		return "386"
	default:
		return strings.ToLower(goarch)
	}
}

// NormalizeVersion strips the "v" prefix from a version tag.
func NormalizeVersion(tag string) string {
	return strings.TrimPrefix(tag, "v")
}

// VersionFromTag extracts and normalizes a version from a git tag.
func VersionFromTag(tag string) string {
	return NormalizeVersion(tag)
}
