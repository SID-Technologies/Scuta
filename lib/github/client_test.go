package github

import (
	"net/http"
	"testing"
)

func TestNormalizeVersion(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"v1.2.3", "1.2.3"},
		{"1.2.3", "1.2.3"},
		{"v0.1.0", "0.1.0"},
		{"", ""},
		{"v", ""},
		{"vv1.0", "v1.0"},
	}

	for _, tt := range tests {
		result := NormalizeVersion(tt.input)
		if result != tt.expected {
			t.Errorf("NormalizeVersion(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestVersionFromTag(t *testing.T) {
	if got := VersionFromTag("v1.0.0"); got != "1.0.0" {
		t.Errorf("VersionFromTag(v1.0.0) = %q, want %q", got, "1.0.0")
	}
}

func TestFindAsset(t *testing.T) {
	assets := []Asset{
		{Name: "pilum_darwin_amd64.tar.gz", BrowserDownloadURL: "https://example.com/darwin_amd64"},
		{Name: "pilum_darwin_arm64.tar.gz", BrowserDownloadURL: "https://example.com/darwin_arm64"},
		{Name: "pilum_linux_amd64.tar.gz", BrowserDownloadURL: "https://example.com/linux_amd64"},
		{Name: "pilum_linux_arm64.tar.gz", BrowserDownloadURL: "https://example.com/linux_arm64"},
		{Name: "pilum_windows_amd64.zip", BrowserDownloadURL: "https://example.com/windows_amd64"},
		{Name: "checksums.txt", BrowserDownloadURL: "https://example.com/checksums"},
	}

	tests := []struct {
		name      string
		os        string
		arch      string
		wantName  string
		wantError bool
	}{
		{"darwin amd64", "darwin", "amd64", "pilum_darwin_amd64.tar.gz", false},
		{"darwin arm64", "darwin", "arm64", "pilum_darwin_arm64.tar.gz", false},
		{"linux amd64", "linux", "amd64", "pilum_linux_amd64.tar.gz", false},
		{"linux arm64", "linux", "arm64", "pilum_linux_arm64.tar.gz", false},
		{"windows amd64", "windows", "amd64", "pilum_windows_amd64.zip", false},
		{"freebsd amd64", "freebsd", "amd64", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			asset, err := FindAsset(assets, tt.os, tt.arch)
			if tt.wantError {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if asset.Name != tt.wantName {
				t.Errorf("got asset %q, want %q", asset.Name, tt.wantName)
			}
		})
	}
}

func TestFindAssetDashSeparator(t *testing.T) {
	assets := []Asset{
		{Name: "tool-darwin-arm64.tar.gz", BrowserDownloadURL: "https://example.com/1"},
		{Name: "tool-linux-amd64.tar.gz", BrowserDownloadURL: "https://example.com/2"},
	}

	asset, err := FindAsset(assets, "darwin", "arm64")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if asset.Name != "tool-darwin-arm64.tar.gz" {
		t.Errorf("got %q, want %q", asset.Name, "tool-darwin-arm64.tar.gz")
	}
}

func TestFindAssetEmpty(t *testing.T) {
	_, err := FindAsset(nil, "darwin", "arm64")
	if err == nil {
		t.Errorf("expected error for empty assets, got nil")
	}
}

func TestFindAssetCaseInsensitive(t *testing.T) {
	assets := []Asset{
		{Name: "Pilum_Darwin_Arm64.tar.gz", BrowserDownloadURL: "https://example.com/1"},
	}

	asset, err := FindAsset(assets, "darwin", "arm64")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if asset.Name != "Pilum_Darwin_Arm64.tar.gz" {
		t.Errorf("got %q, want %q", asset.Name, "Pilum_Darwin_Arm64.tar.gz")
	}
}

func TestNormalizeArch(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"amd64", "amd64"},
		{"x86_64", "amd64"},
		{"arm64", "arm64"},
		{"aarch64", "arm64"},
		{"386", "386"},
		{"i386", "386"},
		{"i686", "386"},
		{"mips", "mips"},
	}

	for _, tt := range tests {
		result := normalizeArch(tt.input)
		if result != tt.expected {
			t.Errorf("normalizeArch(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestNewClient(t *testing.T) {
	client := NewClient("test-token")
	if client.token != "test-token" {
		t.Errorf("expected token 'test-token', got %q", client.token)
	}
	if client.baseURL != "https://api.github.com" {
		t.Errorf("expected baseURL 'https://api.github.com', got %q", client.baseURL)
	}

	client2 := NewClient("")
	if client2.token != "" {
		t.Errorf("expected empty token, got %q", client2.token)
	}
}

func TestValidateDownloadURL(t *testing.T) {
	client := NewClient("")

	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"github release", "https://github.com/owner/repo/releases/download/v1.0/tool.tar.gz", false},
		{"github objects", "https://objects.githubusercontent.com/github-production-release-asset/12345", false},
		{"github subdomain", "https://api.github.com/repos/owner/repo", false},
		{"evil host", "https://evil.com/tool.tar.gz", true},
		{"http not https", "http://github.com/owner/repo/releases/download/v1.0/tool.tar.gz", true},
		{"ftp scheme", "ftp://github.com/file", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := client.validateDownloadURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateDownloadURL(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
			}
		})
	}
}

func TestValidateDownloadURLGitHubEnterprise(t *testing.T) {
	client := NewClient("")
	client.SetBaseURL("https://github.example.com/api/v3")

	// Should accept downloads from the enterprise host
	err := client.validateDownloadURL("https://github.example.com/owner/repo/releases/download/v1.0/tool.tar.gz")
	if err != nil {
		t.Errorf("expected enterprise URL to be valid, got: %v", err)
	}

	// Should still accept standard github.com
	err = client.validateDownloadURL("https://github.com/owner/repo/releases/download/v1.0/tool.tar.gz")
	if err != nil {
		t.Errorf("expected github.com to be valid, got: %v", err)
	}

	// Should reject other hosts
	err = client.validateDownloadURL("https://evil.com/tool.tar.gz")
	if err == nil {
		t.Error("expected error for evil host, got nil")
	}
}

func TestValidateJSONContentType(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		wantErr     bool
	}{
		{"application/json", "application/json", false},
		{"with charset", "application/json; charset=utf-8", false},
		{"text/html", "text/html", true},
		{"text/plain", "text/plain", true},
		{"empty", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &http.Response{
				Header: make(http.Header),
			}
			if tt.contentType != "" {
				resp.Header.Set("Content-Type", tt.contentType)
			}
			err := validateJSONContentType(resp)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateJSONContentType(%q) error = %v, wantErr %v", tt.contentType, err, tt.wantErr)
			}
		})
	}
}
