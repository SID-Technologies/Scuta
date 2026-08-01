package github

import (
	"testing"
)

func TestResolveAsset_Template(t *testing.T) {
	assets := []Asset{
		{Name: "fzf-0.54.0-darwin_arm64.tar.gz"},
		{Name: "fzf-0.54.0-darwin_amd64.tar.gz"},
		{Name: "fzf-0.54.0-linux_amd64.tar.gz"},
	}

	opts := AssetOptions{
		Template: "fzf-{{.Version}}-{{.OS}}_{{.Arch}}.tar.gz",
		Version:  "0.54.0",
		ToolName: "fzf",
	}

	asset, err := ResolveAsset(assets, "darwin", "arm64", opts)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if asset.Name != "fzf-0.54.0-darwin_arm64.tar.gz" {
		t.Fatalf("expected fzf-0.54.0-darwin_arm64.tar.gz, got %s", asset.Name)
	}
}

func TestResolveAsset_TemplateWithMaps(t *testing.T) {
	assets := []Asset{
		{Name: "ripgrep-14.1.1-x86_64-unknown-linux-musl.tar.gz"},
		{Name: "ripgrep-14.1.1-aarch64-unknown-linux-gnu.tar.gz"},
		{Name: "ripgrep-14.1.1-x86_64-apple-darwin.tar.gz"},
	}

	opts := AssetOptions{
		Template: "ripgrep-{{.Version}}-{{.Arch}}-apple-{{.OS}}.tar.gz",
		Version:  "14.1.1",
		ToolName: "ripgrep",
		OSMap:    map[string]string{"darwin": "darwin", "linux": "linux"},
		ArchMap:  map[string]string{"amd64": "x86_64", "arm64": "aarch64"},
	}

	asset, err := ResolveAsset(assets, "darwin", "amd64", opts)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if asset.Name != "ripgrep-14.1.1-x86_64-apple-darwin.tar.gz" {
		t.Fatalf("expected ripgrep-14.1.1-x86_64-apple-darwin.tar.gz, got %s", asset.Name)
	}
}

func TestResolveAsset_FallbackToFindAsset(t *testing.T) {
	assets := []Asset{
		{Name: "pilum_darwin_arm64.tar.gz"},
		{Name: "pilum_linux_amd64.tar.gz"},
	}

	// No template — should fall back to FindAsset
	opts := AssetOptions{
		ToolName: "pilum",
	}

	asset, err := ResolveAsset(assets, "darwin", "arm64", opts)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if asset.Name != "pilum_darwin_arm64.tar.gz" {
		t.Fatalf("expected pilum_darwin_arm64.tar.gz, got %s", asset.Name)
	}
}

func TestResolveAsset_NoMatch(t *testing.T) {
	assets := []Asset{
		{Name: "tool-linux-amd64.tar.gz"},
	}

	opts := AssetOptions{
		Template: "tool-{{.OS}}-{{.Arch}}.tar.gz",
		Version:  "1.0.0",
		ToolName: "tool",
	}

	_, err := ResolveAsset(assets, "darwin", "arm64", opts)
	if err == nil {
		t.Fatal("expected error for no match")
	}
}

func TestFindAssetHeuristic_GoReleaserFirst(t *testing.T) {
	assets := []Asset{
		{Name: "tool_darwin_arm64.tar.gz"},
		{Name: "tool_linux_amd64.tar.gz"},
	}

	asset, err := FindAssetHeuristic(assets, "darwin", "arm64")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if asset.Name != "tool_darwin_arm64.tar.gz" {
		t.Fatalf("expected tool_darwin_arm64.tar.gz, got %s", asset.Name)
	}
}

func TestFindAssetHeuristic_SubstringMatch(t *testing.T) {
	// Non-standard naming but contains OS and arch substrings
	assets := []Asset{
		{Name: "checksums.txt"},
		{Name: "mytool-v1.0.0-macos-x86_64.tar.gz"},
		{Name: "mytool-v1.0.0-linux-x86_64.tar.gz"},
	}

	asset, err := FindAssetHeuristic(assets, "darwin", "amd64")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if asset.Name != "mytool-v1.0.0-macos-x86_64.tar.gz" {
		t.Fatalf("expected mytool-v1.0.0-macos-x86_64.tar.gz, got %s", asset.Name)
	}
}

func TestFindAssetHeuristic_SingleOSMatch(t *testing.T) {
	// Only one archive matches the OS — pick it even without arch match
	assets := []Asset{
		{Name: "checksums.txt"},
		{Name: "mytool-darwin.tar.gz"},
		{Name: "mytool-linux.tar.gz"},
	}

	asset, err := FindAssetHeuristic(assets, "darwin", "arm64")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if asset.Name != "mytool-darwin.tar.gz" {
		t.Fatalf("expected mytool-darwin.tar.gz, got %s", asset.Name)
	}
}

func TestFindAssetHeuristic_NoMatch(t *testing.T) {
	assets := []Asset{
		{Name: "mytool-freebsd-amd64.tar.gz"},
	}

	_, err := FindAssetHeuristic(assets, "darwin", "arm64")
	if err == nil {
		t.Fatal("expected error for no match")
	}
}

func TestFindAssetHeuristic_EmptyAssets(t *testing.T) {
	_, err := FindAssetHeuristic(nil, "darwin", "arm64")
	if err == nil {
		t.Fatal("expected error for empty assets")
	}
}

func TestIsRawBinary(t *testing.T) {
	tests := []struct {
		name     string
		expected bool
	}{
		{"jq-linux-amd64", true},
		{"tool.tar.gz", false},
		{"tool.tgz", false},
		{"tool.zip", false},
		{"jq-osx-amd64", true},
		{"checksums.txt", true},
	}

	for _, tt := range tests {
		got := IsRawBinary(tt.name)
		if got != tt.expected {
			t.Errorf("IsRawBinary(%q) = %v, want %v", tt.name, got, tt.expected)
		}
	}
}

func TestResolveMapping(t *testing.T) {
	mapping := map[string]string{
		"darwin": "Darwin",
		"linux":  "Linux",
	}

	if got := resolveMapping("darwin", mapping); got != "Darwin" {
		t.Errorf("expected Darwin, got %s", got)
	}

	if got := resolveMapping("windows", mapping); got != "windows" {
		t.Errorf("expected windows (unmapped), got %s", got)
	}

	if got := resolveMapping("darwin", nil); got != "darwin" {
		t.Errorf("expected darwin (nil map), got %s", got)
	}
}

func TestOsAliases(t *testing.T) {
	aliases := osAliasesFor("darwin")
	if len(aliases) < 2 {
		t.Fatalf("expected multiple aliases for darwin, got %v", aliases)
	}

	found := false
	for _, a := range aliases {
		if a == "macos" {
			found = true
		}
	}
	if !found {
		t.Error("expected 'macos' in darwin aliases")
	}
}

func TestArchAliases(t *testing.T) {
	aliases := archAliasesFor("amd64")
	if len(aliases) < 2 {
		t.Fatalf("expected multiple aliases for amd64, got %v", aliases)
	}

	found := false
	for _, a := range aliases {
		if a == "x86_64" {
			found = true
		}
	}
	if !found {
		t.Error("expected 'x86_64' in amd64 aliases")
	}
}

func TestResolveAsset_EmptyTemplateUsesHeuristic(t *testing.T) {
	// No template: ResolveAsset must fall back to the heuristic matcher so
	// Rust-style target triples (arch-first, "aarch64", "apple") still resolve.
	// Regression for direct-installed tools failing on `scuta update`.
	assets := []Asset{
		{Name: "bat-v0.26.1-x86_64-unknown-linux-gnu.tar.gz"},
		{Name: "bat-v0.26.1-aarch64-apple-darwin.tar.gz"},
		{Name: "bat_0.26.1_arm64.deb"},
	}

	asset, err := ResolveAsset(assets, "darwin", "arm64", AssetOptions{})
	if err != nil {
		t.Fatalf("expected empty-template resolution to succeed, got %v", err)
	}
	if asset.Name != "bat-v0.26.1-aarch64-apple-darwin.tar.gz" {
		t.Fatalf("expected bat-v0.26.1-aarch64-apple-darwin.tar.gz, got %s", asset.Name)
	}
}

// fzf055Assets is the real asset list of junegunn/fzf v0.55.0. It mixes
// "-" and "_" separators and its darwin assets contain the substring "win",
// which historically caused the darwin binary to be installed on Windows.
func fzf055Assets() []Asset {
	names := []string{
		"fzf-0.55.0-darwin_amd64.tar.gz",
		"fzf-0.55.0-darwin_arm64.tar.gz",
		"fzf-0.55.0-freebsd_amd64.tar.gz",
		"fzf-0.55.0-linux_amd64.tar.gz",
		"fzf-0.55.0-linux_arm64.tar.gz",
		"fzf-0.55.0-linux_armv5.tar.gz",
		"fzf-0.55.0-linux_armv6.tar.gz",
		"fzf-0.55.0-linux_armv7.tar.gz",
		"fzf-0.55.0-linux_loong64.tar.gz",
		"fzf-0.55.0-linux_ppc64le.tar.gz",
		"fzf-0.55.0-linux_s390x.tar.gz",
		"fzf-0.55.0-openbsd_amd64.tar.gz",
		"fzf-0.55.0-windows_amd64.zip",
		"fzf-0.55.0-windows_arm64.zip",
		"fzf-0.55.0-windows_armv5.zip",
		"fzf-0.55.0-windows_armv6.zip",
		"fzf-0.55.0-windows_armv7.zip",
		"fzf_0.55.0_checksums.txt",
	}
	assets := make([]Asset, 0, len(names))
	for _, n := range names {
		assets = append(assets, Asset{Name: n})
	}
	return assets
}

func TestFindAssetHeuristic_MixedSeparators(t *testing.T) {
	// Regression: "win" alias must not match inside "darwin", and mixed
	// -/_ separators must resolve to the exact platform asset.
	cases := []struct {
		goos, goarch, want string
	}{
		{"windows", "amd64", "fzf-0.55.0-windows_amd64.zip"},
		{"windows", "arm64", "fzf-0.55.0-windows_arm64.zip"},
		{"darwin", "amd64", "fzf-0.55.0-darwin_amd64.tar.gz"},
		{"darwin", "arm64", "fzf-0.55.0-darwin_arm64.tar.gz"},
		{"linux", "amd64", "fzf-0.55.0-linux_amd64.tar.gz"},
		{"linux", "arm64", "fzf-0.55.0-linux_arm64.tar.gz"},
	}

	for _, tc := range cases {
		asset, err := FindAssetHeuristic(fzf055Assets(), tc.goos, tc.goarch)
		if err != nil {
			t.Fatalf("%s/%s: unexpected error: %v", tc.goos, tc.goarch, err)
		}
		if asset.Name != tc.want {
			t.Errorf("%s/%s: got %s, want %s", tc.goos, tc.goarch, asset.Name, tc.want)
		}
	}
}

func TestContainsToken(t *testing.T) {
	cases := []struct {
		s, token string
		want     bool
	}{
		{"fzf-0.55.0-darwin_amd64.tar.gz", "win", false},
		{"fzf-0.55.0-windows_amd64.zip", "windows", true},
		{"tool-win64.zip", "win64", true},
		{"tool-win-x64.zip", "win", true},
		{"tool-win-x64.zip", "x64", true},
		{"tool_linux_amd64.tar.gz", "amd64", true},
		{"tool_linux_arm64.tar.gz", "amd64", false},
		{"windows.zip", "windows", true},
		{"anything", "", false},
	}

	for _, tc := range cases {
		if got := containsToken(tc.s, tc.token); got != tc.want {
			t.Errorf("containsToken(%q, %q) = %v, want %v", tc.s, tc.token, got, tc.want)
		}
	}
}
