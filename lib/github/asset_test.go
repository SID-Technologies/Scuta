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
