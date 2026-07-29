package manifest

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_ShorthandAndFullForms(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "scuta.lock.yaml")
	content := `tools:
  pilum: "0.7.5"
  ripgrep:
    version: "15.2.0"
    repo: "BurntSushi/ripgrep"
  api-gen: "latest"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	m, err := Load(path)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	if m.Tools["pilum"].Version != "0.7.5" {
		t.Errorf("pilum: expected shorthand version 0.7.5, got %q", m.Tools["pilum"].Version)
	}
	if m.Tools["ripgrep"].Repo != "BurntSushi/ripgrep" {
		t.Errorf("ripgrep: expected repo override, got %q", m.Tools["ripgrep"].Repo)
	}
	if m.Tools["ripgrep"].Version != "15.2.0" {
		t.Errorf("ripgrep: expected version 15.2.0, got %q", m.Tools["ripgrep"].Version)
	}
	if m.Tools["api-gen"].Version != "latest" {
		t.Errorf("api-gen: expected latest, got %q", m.Tools["api-gen"].Version)
	}
}

func TestLoad_EmptyManifestErrors(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "scuta.lock.yaml")
	if err := os.WriteFile(path, []byte("tools: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Error("expected error for empty manifest, got nil")
	}
}

func TestFindDefault(t *testing.T) {
	dir := t.TempDir()
	if got := FindDefault(dir); got != "" {
		t.Errorf("expected no manifest, got %q", got)
	}
	path := filepath.Join(dir, "scuta.yaml")
	if err := os.WriteFile(path, []byte("tools:\n  x: latest\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := FindDefault(dir); got != path {
		t.Errorf("expected %q, got %q", path, got)
	}
}

func TestNormalizeVersion(t *testing.T) {
	cases := []struct {
		in       string
		wantNorm string
		wantLat  bool
	}{
		{"0.7.5", "0.7.5", false},
		{"v1.2.3", "1.2.3", false},
		{"latest", "", true},
		{"LATEST", "", true},
		{"", "", true},
		{"  v2.0.0  ", "2.0.0", false},
	}
	for _, c := range cases {
		gotNorm, gotLat := NormalizeVersion(c.in)
		if gotNorm != c.wantNorm || gotLat != c.wantLat {
			t.Errorf("NormalizeVersion(%q) = (%q, %v), want (%q, %v)",
				c.in, gotNorm, gotLat, c.wantNorm, c.wantLat)
		}
	}
}

func TestLoad_BinField(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "scuta.lock.yaml")
	content := `tools:
  ripgrep:
    version: "14.1.0"
    repo: "BurntSushi/ripgrep"
    bin: "rg"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	m, err := Load(path)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if got := m.Tools["ripgrep"].Bin; got != "rg" {
		t.Errorf("expected bin override rg, got %q", got)
	}
}
