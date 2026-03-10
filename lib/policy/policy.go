// Package policy enforces version pinning, blocking, and minimum Scuta version requirements.
// Policies are loaded from ~/.scuta/policy.yaml or a remote URL.
package policy

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/sid-technologies/scuta/lib/errors"

	"github.com/Masterminds/semver/v3"
	"gopkg.in/yaml.v3"
)

const policyFile = "policy.yaml"

// Policy defines version constraints and blocked versions for tools.
type Policy struct {
	// MinScutaVersion is the minimum Scuta version required by this policy.
	MinScutaVersion string `yaml:"min_scuta_version,omitempty"`

	// Tools maps tool names to their version policies.
	Tools map[string]ToolPolicy `yaml:"tools,omitempty"`
}

// ToolPolicy defines version constraints for a single tool.
type ToolPolicy struct {
	// Allowed is a semver constraint string (e.g. ">=1.0.0", "~1.2.0").
	Allowed string `yaml:"allowed,omitempty"`

	// Blocked is a list of specific versions that are not allowed.
	Blocked []string `yaml:"blocked,omitempty"`
}

// Violation describes a policy rule that was violated.
type Violation struct {
	Tool    string
	Version string
	Rule    string
	Message string
}

// Parse unmarshals YAML data into a Policy.
func Parse(data []byte) (*Policy, error) {
	var p Policy
	if err := yaml.Unmarshal(data, &p); err != nil {
		return nil, errors.Wrap(err, "parsing policy file")
	}

	return &p, nil
}

// Load reads the policy from ~/.scuta/policy.yaml.
// Returns nil if the file does not exist.
func Load(scutaDir string) (*Policy, error) {
	fp := filepath.Join(scutaDir, policyFile)

	data, err := os.ReadFile(fp)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, errors.Wrap(err, "reading policy file")
	}

	return Parse(data)
}

// FetchRemote downloads a policy from the given URL.
func FetchRemote(url string) (*Policy, error) {
	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
		},
	}

	resp, err := client.Get(url) //nolint:noctx // simple GET with timeout
	if err != nil {
		return nil, errors.Wrap(err, "fetching remote policy")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("remote policy returned %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "reading remote policy response")
	}

	return Parse(data)
}

// CheckToolVersion checks whether a tool version satisfies the policy.
// Returns nil if no violation is found.
func (p *Policy) CheckToolVersion(toolName, version string) *Violation {
	if p == nil {
		return nil
	}

	tp, ok := p.Tools[toolName]
	if !ok {
		return nil
	}

	// Check blocked list first
	for _, blocked := range tp.Blocked {
		if blocked == version {
			return &Violation{
				Tool:    toolName,
				Version: version,
				Rule:    "blocked",
				Message: fmt.Sprintf("version %s is blocked by policy", version),
			}
		}
	}

	// Check allowed constraint
	if tp.Allowed == "" {
		return nil
	}

	constraint, err := semver.NewConstraint(tp.Allowed)
	if err != nil {
		return &Violation{
			Tool:    toolName,
			Version: version,
			Rule:    "invalid_constraint",
			Message: fmt.Sprintf("invalid policy constraint %q: %v", tp.Allowed, err),
		}
	}

	v, err := semver.NewVersion(version)
	if err != nil {
		// Can't parse version — skip constraint check
		return nil
	}

	if !constraint.Check(v) {
		return &Violation{
			Tool:    toolName,
			Version: version,
			Rule:    "not_allowed",
			Message: fmt.Sprintf("version %s does not satisfy constraint %q", version, tp.Allowed),
		}
	}

	return nil
}

// CheckScutaVersion checks whether the current Scuta version meets the minimum.
// Returns nil if no violation is found.
func (p *Policy) CheckScutaVersion(currentVersion string) *Violation {
	if p == nil || p.MinScutaVersion == "" {
		return nil
	}

	minVer, err := semver.NewVersion(p.MinScutaVersion)
	if err != nil {
		return &Violation{
			Tool:    "scuta",
			Version: currentVersion,
			Rule:    "invalid_constraint",
			Message: fmt.Sprintf("invalid min_scuta_version %q: %v", p.MinScutaVersion, err),
		}
	}

	cur, err := semver.NewVersion(currentVersion)
	if err != nil {
		// Can't parse current version — skip check
		return nil
	}

	if cur.LessThan(minVer) {
		return &Violation{
			Tool:    "scuta",
			Version: currentVersion,
			Rule:    "min_version",
			Message: fmt.Sprintf("scuta %s is below minimum required version %s", currentVersion, p.MinScutaVersion),
		}
	}

	return nil
}

// CheckAll checks all installed tools against the policy.
// Returns a list of violations (empty if all tools comply).
func (p *Policy) CheckAll(installed map[string]string) []Violation {
	if p == nil {
		return nil
	}

	var violations []Violation
	for toolName, version := range installed {
		if v := p.CheckToolVersion(toolName, version); v != nil {
			violations = append(violations, *v)
		}
	}

	return violations
}
