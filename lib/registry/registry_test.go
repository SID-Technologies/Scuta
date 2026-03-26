package registry

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMerge_OverlayWinsOnConflict(t *testing.T) {
	base := &Registry{
		Tools: map[string]Tool{
			"pilum": {Description: "Original", Repo: "sid/pilum"},
			"other": {Description: "Stays", Repo: "sid/other"},
		},
		Sources: map[string]string{
			"pilum": SourceRemote,
			"other": SourceRemote,
		},
	}

	overlay := &Registry{
		Tools: map[string]Tool{
			"pilum": {Description: "Forked", Repo: "my-org/pilum"},
		},
		Sources: map[string]string{
			"pilum": SourceLocal,
		},
	}

	Merge(base, overlay)

	// Overlay tool should win
	if base.Tools["pilum"].Repo != "my-org/pilum" {
		t.Errorf("expected overlay repo, got %s", base.Tools["pilum"].Repo)
	}
	if base.Tools["pilum"].Description != "Forked" {
		t.Errorf("expected overlay description, got %s", base.Tools["pilum"].Description)
	}

	// Source should be updated
	if base.Sources["pilum"] != SourceLocal {
		t.Errorf("expected source %q, got %q", SourceLocal, base.Sources["pilum"])
	}

	// Non-conflicting tool should remain
	if base.Tools["other"].Repo != "sid/other" {
		t.Errorf("expected base repo, got %s", base.Tools["other"].Repo)
	}
	if base.Sources["other"] != SourceRemote {
		t.Errorf("expected source %q, got %q", SourceRemote, base.Sources["other"])
	}
}

func TestMerge_BothToolsPresent(t *testing.T) {
	base := &Registry{
		Tools: map[string]Tool{
			"api-gen": {Description: "API gen", Repo: "sid/api-gen"},
		},
		Sources: map[string]string{
			"api-gen": SourceRemote,
		},
	}

	overlay := &Registry{
		Tools: map[string]Tool{
			"my-tool": {Description: "My tool", Repo: "my-org/my-tool"},
		},
		Sources: map[string]string{
			"my-tool": SourceLocal,
		},
	}

	Merge(base, overlay)

	if len(base.Tools) != 2 {
		t.Errorf("expected 2 tools, got %d", len(base.Tools))
	}
	if _, ok := base.Tools["api-gen"]; !ok {
		t.Error("expected api-gen to be present")
	}
	if _, ok := base.Tools["my-tool"]; !ok {
		t.Error("expected my-tool to be present")
	}
}

func TestMerge_NilSources(t *testing.T) {
	base := &Registry{
		Tools: map[string]Tool{
			"existing": {Repo: "sid/existing"},
		},
	}

	overlay := &Registry{
		Tools: map[string]Tool{
			"new-tool": {Repo: "org/new-tool"},
		},
	}

	Merge(base, overlay)

	if base.Sources == nil {
		t.Fatal("expected sources to be initialized")
	}
	if base.Sources["new-tool"] != SourceLocal {
		t.Errorf("expected source %q, got %q", SourceLocal, base.Sources["new-tool"])
	}
}

func TestLoadLocal_FileExists(t *testing.T) {
	dir := t.TempDir()
	content := []byte(`tools:
  my-tool:
    description: "Test tool"
    repo: my-org/my-tool
  another:
    description: "Another tool"
    repo: my-org/another
    depends_on:
      - my-tool
`)
	if err := os.WriteFile(filepath.Join(dir, localFile), content, 0o600); err != nil {
		t.Fatal(err)
	}

	reg, err := loadLocal(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(reg.Tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(reg.Tools))
	}

	tool, ok := reg.Get("my-tool")
	if !ok {
		t.Fatal("expected my-tool to exist")
	}
	if tool.Repo != "my-org/my-tool" {
		t.Errorf("expected repo my-org/my-tool, got %s", tool.Repo)
	}

	another, ok := reg.Get("another")
	if !ok {
		t.Fatal("expected another to exist")
	}
	if len(another.DependsOn) != 1 || another.DependsOn[0] != "my-tool" {
		t.Errorf("expected depends_on [my-tool], got %v", another.DependsOn)
	}

	// Check sources are tagged
	if reg.Sources["my-tool"] != SourceLocal {
		t.Errorf("expected source %q, got %q", SourceLocal, reg.Sources["my-tool"])
	}
}

func TestLoadLocal_FileNotExists(t *testing.T) {
	dir := t.TempDir()

	reg, err := loadLocal(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(reg.Tools) != 0 {
		t.Errorf("expected empty registry, got %d tools", len(reg.Tools))
	}
}

func TestLoadLocal_MalformedYAML(t *testing.T) {
	dir := t.TempDir()
	content := []byte(`tools:
  broken: [this is not valid yaml for a tool
`)
	if err := os.WriteFile(filepath.Join(dir, localFile), content, 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := loadLocal(dir)
	if err == nil {
		t.Fatal("expected error for malformed YAML")
	}
}

func TestSaveLocal_RoundTrip(t *testing.T) {
	dir := t.TempDir()

	original := &Registry{
		Tools: map[string]Tool{
			"my-tool": {
				Description: "Test tool",
				Repo:        "my-org/my-tool",
			},
			"dep-tool": {
				Description: "Has deps",
				Repo:        "my-org/dep-tool",
				DependsOn:   []string{"my-tool"},
			},
		},
	}

	if err := SaveLocal(dir, original); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	loaded, err := loadLocal(dir)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	if len(loaded.Tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(loaded.Tools))
	}

	tool, ok := loaded.Get("my-tool")
	if !ok {
		t.Fatal("expected my-tool to exist")
	}
	if tool.Repo != "my-org/my-tool" {
		t.Errorf("expected repo my-org/my-tool, got %s", tool.Repo)
	}
	if tool.Description != "Test tool" {
		t.Errorf("expected description 'Test tool', got %s", tool.Description)
	}

	dep, ok := loaded.Get("dep-tool")
	if !ok {
		t.Fatal("expected dep-tool to exist")
	}
	if len(dep.DependsOn) != 1 || dep.DependsOn[0] != "my-tool" {
		t.Errorf("expected depends_on [my-tool], got %v", dep.DependsOn)
	}
}

func TestSource(t *testing.T) {
	reg := &Registry{
		Tools: map[string]Tool{
			"a": {Repo: "org/a"},
			"b": {Repo: "org/b"},
		},
		Sources: map[string]string{
			"a": SourceRemote,
			"b": SourceLocal,
		},
	}

	if s := reg.Source("a"); s != SourceRemote {
		t.Errorf("expected %q, got %q", SourceRemote, s)
	}
	if s := reg.Source("b"); s != SourceLocal {
		t.Errorf("expected %q, got %q", SourceLocal, s)
	}
	if s := reg.Source("missing"); s != "" {
		t.Errorf("expected empty, got %q", s)
	}
}

func TestParse_ExtendedFields(t *testing.T) {
	data := []byte(`tools:
  ripgrep:
    description: "Search tool"
    repo: BurntSushi/ripgrep
    bin: rg
    asset: "ripgrep-{{.Version}}-{{.Arch}}-{{.OS}}.tar.gz"
    version_prefix: "none"
    os_map:
      darwin: apple-darwin
      linux: unknown-linux-musl
    arch_map:
      amd64: x86_64
      arm64: aarch64
  fzf:
    description: "Fuzzy finder"
    repo: junegunn/fzf
    asset: "fzf-{{.Version}}-{{.OS}}_{{.Arch}}.tar.gz"
`)

	reg, err := parse(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	rg, ok := reg.Get("ripgrep")
	if !ok {
		t.Fatal("expected ripgrep to exist")
	}
	if rg.Bin != "rg" {
		t.Errorf("expected bin 'rg', got %q", rg.Bin)
	}
	if rg.VersionPrefix != "none" {
		t.Errorf("expected version_prefix 'none', got %q", rg.VersionPrefix)
	}
	if rg.OSMap["darwin"] != "apple-darwin" {
		t.Errorf("expected os_map darwin=apple-darwin, got %q", rg.OSMap["darwin"])
	}
	if rg.ArchMap["amd64"] != "x86_64" {
		t.Errorf("expected arch_map amd64=x86_64, got %q", rg.ArchMap["amd64"])
	}
	if rg.Asset == "" {
		t.Error("expected asset template to be set")
	}

	fzf, ok := reg.Get("fzf")
	if !ok {
		t.Fatal("expected fzf to exist")
	}
	if fzf.Bin != "" {
		t.Errorf("expected empty bin, got %q", fzf.Bin)
	}
	if fzf.VersionPrefix != "" {
		t.Errorf("expected empty version_prefix, got %q", fzf.VersionPrefix)
	}
	if len(fzf.OSMap) != 0 {
		t.Errorf("expected empty os_map, got %v", fzf.OSMap)
	}
}

func TestSaveLocal_ExtendedFields(t *testing.T) {
	dir := t.TempDir()

	original := &Registry{
		Tools: map[string]Tool{
			"ripgrep": {
				Description:   "Search tool",
				Repo:          "BurntSushi/ripgrep",
				Bin:           "rg",
				Asset:         "ripgrep-{{.Version}}-{{.Arch}}-{{.OS}}.tar.gz",
				VersionPrefix: "none",
				OSMap:         map[string]string{"darwin": "apple-darwin"},
				ArchMap:       map[string]string{"amd64": "x86_64"},
			},
		},
	}

	if err := SaveLocal(dir, original); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	loaded, err := loadLocal(dir)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	tool, ok := loaded.Get("ripgrep")
	if !ok {
		t.Fatal("expected ripgrep to exist")
	}
	if tool.Bin != "rg" {
		t.Errorf("expected bin 'rg', got %q", tool.Bin)
	}
	if tool.VersionPrefix != "none" {
		t.Errorf("expected version_prefix 'none', got %q", tool.VersionPrefix)
	}
	if tool.OSMap["darwin"] != "apple-darwin" {
		t.Errorf("expected os_map darwin=apple-darwin, got %q", tool.OSMap["darwin"])
	}
	if tool.ArchMap["amd64"] != "x86_64" {
		t.Errorf("expected arch_map amd64=x86_64, got %q", tool.ArchMap["amd64"])
	}
}

func TestSource_NilSources(t *testing.T) {
	reg := &Registry{
		Tools: map[string]Tool{
			"a": {Repo: "org/a"},
		},
	}

	if s := reg.Source("a"); s != "" {
		t.Errorf("expected empty, got %q", s)
	}
}
