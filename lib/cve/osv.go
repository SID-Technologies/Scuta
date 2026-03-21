// Package cve provides vulnerability checking via the OSV.dev API.
package cve

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/sid-technologies/scuta/lib/errors"
)

const (
	osvAPIURL   = "https://api.osv.dev/v1/query"
	cacheFile   = "cve_cache.json"
	cacheTTL    = 24 * time.Hour
	maxRespSize = 5 * 1024 * 1024 // 5 MB
)

// Vuln represents a vulnerability from the OSV database.
type Vuln struct {
	ID       string   `json:"id"`
	Summary  string   `json:"summary"`
	Severity string   `json:"severity,omitempty"`
	Aliases  []string `json:"aliases,omitempty"`
}

// osvQueryRequest is the request payload for the OSV API.
type osvQueryRequest struct {
	Package osvPackage `json:"package"`
	Version string     `json:"version"`
}

// osvPackage identifies a package for the OSV query.
type osvPackage struct {
	Name      string `json:"name"`
	Ecosystem string `json:"ecosystem"`
}

// osvQueryResponse is the response from the OSV API.
type osvQueryResponse struct {
	Vulns []osvVuln `json:"vulns"`
}

// osvVuln is a vulnerability entry from the OSV API response.
type osvVuln struct {
	ID       string        `json:"id"`
	Summary  string        `json:"summary"`
	Aliases  []string      `json:"aliases"`
	Severity []osvSeverity `json:"severity"`
}

// osvSeverity holds severity info from the OSV response.
type osvSeverity struct {
	Type  string `json:"type"`
	Score string `json:"score"`
}

// cacheEntry holds cached CVE results for a single tool.
type cacheEntry struct {
	Vulns     []Vuln    `json:"vulns"`
	CheckedAt time.Time `json:"checked_at"`
}

// cache holds all cached CVE results.
type cache map[string]cacheEntry // keyed by "name@version"

// CheckVulnerabilities queries the OSV.dev API for known vulnerabilities
// affecting the given package name and version.
// The ecosystem should be "Go" for Go binaries, or "GitHub Actions" for actions, etc.
func CheckVulnerabilities(name string, version string, ecosystem string) ([]Vuln, error) {
	reqBody := osvQueryRequest{
		Package: osvPackage{
			Name:      name,
			Ecosystem: ecosystem,
		},
		Version: version,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, errors.Wrap(err, "marshaling OSV query")
	}

	resp, err := http.Post(osvAPIURL, "application/json", bytes.NewReader(data)) //nolint:gosec,noctx // OSV API URL is constant
	if err != nil {
		return nil, errors.Wrap(err, "querying OSV API")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("OSV API returned %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxRespSize))
	if err != nil {
		return nil, errors.Wrap(err, "reading OSV response")
	}

	var result osvQueryResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, errors.Wrap(err, "parsing OSV response")
	}

	return convertVulns(result.Vulns), nil
}

// CheckWithCache queries the OSV API with local caching.
// Results are cached per tool+version for 24 hours.
func CheckWithCache(scutaDir string, name string, version string, ecosystem string) ([]Vuln, error) {
	cacheKey := name + "@" + version

	// Try cache first
	c, _ := loadCache(scutaDir)
	if entry, ok := c[cacheKey]; ok {
		if time.Since(entry.CheckedAt) < cacheTTL {
			return entry.Vulns, nil
		}
	}

	// Query OSV
	vulns, err := CheckVulnerabilities(name, version, ecosystem)
	if err != nil {
		return nil, err
	}

	// Update cache
	if c == nil {
		c = make(cache)
	}
	c[cacheKey] = cacheEntry{
		Vulns:     vulns,
		CheckedAt: time.Now(),
	}
	_ = saveCache(scutaDir, c)

	return vulns, nil
}

// loadCache reads the CVE cache from disk.
func loadCache(scutaDir string) (cache, error) {
	fp := filepath.Join(scutaDir, cacheFile)

	data, err := os.ReadFile(fp)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, errors.Wrap(err, "reading CVE cache")
	}

	var c cache
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, errors.Wrap(err, "parsing CVE cache")
	}

	return c, nil
}

// saveCache writes the CVE cache to disk.
func saveCache(scutaDir string, c cache) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return errors.Wrap(err, "marshaling CVE cache")
	}

	fp := filepath.Join(scutaDir, cacheFile)
	return os.WriteFile(fp, data, 0o600)
}

// convertVulns converts OSV vulns to our Vuln type.
func convertVulns(osvVulns []osvVuln) []Vuln {
	var vulns []Vuln
	for _, v := range osvVulns {
		severity := ""
		if len(v.Severity) > 0 {
			severity = v.Severity[0].Score
		}
		vulns = append(vulns, Vuln{
			ID:       v.ID,
			Summary:  v.Summary,
			Severity: severity,
			Aliases:  v.Aliases,
		})
	}
	return vulns
}
