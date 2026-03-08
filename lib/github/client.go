// Package github provides a client for the GitHub Releases API.
package github

// Client provides access to GitHub Releases for downloading tool binaries.
type Client struct {
	token string
}

// NewClient creates a GitHub API client with an optional auth token.
func NewClient(token string) *Client {
	return &Client{token: token}
}

// Release represents a GitHub release.
type Release struct {
	TagName string  `json:"tag_name"`
	Assets  []Asset `json:"assets"`
}

// Asset represents a downloadable file attached to a release.
type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

// TODO(phase2): Implement GetLatestRelease, DownloadAsset
