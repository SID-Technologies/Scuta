package github

import (
	"bytes"
	"strings"
	"text/template"
)

// AssetOptions configures asset resolution for a tool.
// When Template is set, the template is rendered and matched against assets.
// When Template is empty, the standard GoReleaser pattern matching is used.
type AssetOptions struct {
	Template string
	OSMap    map[string]string
	ArchMap  map[string]string
	Version  string
	ToolName string
	BinName  string
}

// templateData holds the variables available in asset templates.
type templateData struct {
	Name    string // Tool name
	Version string // Version without "v" prefix
	OS      string // Resolved OS (after OSMap)
	Arch    string // Resolved arch (after ArchMap)
}

// ResolveAsset finds the best matching asset using the provided options.
// If opts.Template is set, it renders the template and matches against assets.
// Otherwise, it falls back to the standard GoReleaser pattern matching via FindAsset.
func ResolveAsset(assets []Asset, goos, goarch string, opts AssetOptions) (*Asset, error) {
	if opts.Template == "" {
		return FindAsset(assets, goos, goarch)
	}

	resolvedOS := resolveMapping(goos, opts.OSMap)
	resolvedArch := resolveMapping(goarch, opts.ArchMap)

	data := templateData{
		Name:    opts.ToolName,
		Version: opts.Version,
		OS:      resolvedOS,
		Arch:    resolvedArch,
	}

	tmpl, err := template.New("asset").Parse(opts.Template)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, err
	}

	expected := buf.String()

	for i := range assets {
		if assets[i].Name == expected {
			return &assets[i], nil
		}
	}

	// No exact match — list available assets in error
	var available []string
	for _, a := range assets {
		available = append(available, a.Name)
	}

	return nil, newNoAssetError(expected, goos, goarch, available)
}

// FindAssetHeuristic uses relaxed matching to find an asset for the given OS and architecture.
// It tries progressively looser strategies:
//  1. Standard GoReleaser pattern matching (via FindAsset)
//  2. Substring matching — find archives containing OS + arch strings
//  3. If only one archive matches the OS, pick it
func FindAssetHeuristic(assets []Asset, goos, goarch string) (*Asset, error) {
	if len(assets) == 0 {
		return nil, newNoAssetError("", goos, goarch, nil)
	}

	// Strategy 1: Standard GoReleaser patterns
	if asset, err := FindAsset(assets, goos, goarch); err == nil {
		return asset, nil
	}

	normalizedOS := normalizeOS(goos)
	normalizedArch := normalizeArch(goarch)

	// Build OS aliases to check (e.g., darwin AND macos AND apple)
	osAliases := osAliasesFor(normalizedOS)
	archAliases := archAliasesFor(normalizedArch)

	// Strategy 2: Substring matching — archive contains OS + arch
	for i := range assets {
		name := strings.ToLower(assets[i].Name)
		if !isArchive(name) {
			continue
		}
		if containsAny(name, osAliases) && containsAny(name, archAliases) {
			return &assets[i], nil
		}
	}

	// Strategy 3: If exactly one archive matches the OS, pick it
	var osMatches []*Asset
	for i := range assets {
		name := strings.ToLower(assets[i].Name)
		if !isArchive(name) {
			continue
		}
		if containsAny(name, osAliases) {
			osMatches = append(osMatches, &assets[i])
		}
	}

	if len(osMatches) == 1 {
		return osMatches[0], nil
	}

	var available []string
	for _, a := range assets {
		available = append(available, a.Name)
	}

	return nil, newNoAssetError("", goos, goarch, available)
}

// resolveMapping applies a user-provided mapping (e.g., OSMap or ArchMap).
// If no mapping exists for the key, the original value is returned.
func resolveMapping(key string, mapping map[string]string) string {
	if mapping == nil {
		return key
	}

	if mapped, ok := mapping[key]; ok {
		return mapped
	}

	return key
}

// isArchive returns true if the filename has a known archive extension.
func isArchive(name string) bool {
	return strings.HasSuffix(name, ".tar.gz") ||
		strings.HasSuffix(name, ".tgz") ||
		strings.HasSuffix(name, ".zip")
}

// IsRawBinary returns true if the asset name does not have a known archive extension.
// Raw binaries are downloaded directly without extraction.
func IsRawBinary(name string) bool {
	return !isArchive(strings.ToLower(name))
}

// osAliasesFor returns common aliases for an OS name.
func osAliasesFor(os string) []string {
	switch os {
	case "darwin":
		return []string{"darwin", "macos", "apple", "osx"}
	case "linux":
		return []string{"linux"}
	case "windows":
		return []string{"windows", "win", "win64", "win32"}
	default:
		return []string{os}
	}
}

// archAliasesFor returns common aliases for an architecture name.
func archAliasesFor(arch string) []string {
	switch arch {
	case "amd64":
		return []string{"amd64", "x86_64", "x64"}
	case "arm64":
		return []string{"arm64", "aarch64"}
	case "386":
		return []string{"386", "i386", "i686", "x86"}
	default:
		return []string{arch}
	}
}

// containsAny returns true if s contains any of the substrings.
func containsAny(s string, substrings []string) bool {
	for _, sub := range substrings {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// newNoAssetError builds a consistent error for when no asset is found.
func newNoAssetError(expected, goos, goarch string, available []string) error {
	if expected != "" {
		return &NoAssetError{
			Expected:  expected,
			GOOS:      goos,
			GOARCH:    goarch,
			Available: available,
		}
	}
	return &NoAssetError{
		GOOS:      goos,
		GOARCH:    goarch,
		Available: available,
	}
}

// NoAssetError is returned when no matching asset is found.
type NoAssetError struct {
	Expected  string
	GOOS      string
	GOARCH    string
	Available []string
}

func (e *NoAssetError) Error() string {
	if e.Expected != "" {
		return "no asset matching " + e.Expected + " for " + e.GOOS + "/" + e.GOARCH +
			". Available: " + strings.Join(e.Available, ", ")
	}
	return "no asset found for " + e.GOOS + "/" + e.GOARCH +
		". Available: " + strings.Join(e.Available, ", ")
}
