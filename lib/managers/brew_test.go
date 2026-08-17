package managers

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/sid-technologies/scuta/lib/audit"
)

// fakeBrew builds a Brew over an in-memory Cellar. dirs maps directory
// path to entries; files maps file path to content.
func fakeBrew(env map[string]string, dirs map[string][]os.DirEntry, files map[string]string) *Brew {
	return &Brew{
		getenv: func(k string) string { return env[k] },
		readDir: func(p string) ([]os.DirEntry, error) {
			if e, ok := dirs[p]; ok {
				return e, nil
			}
			return nil, errors.New("no such dir")
		},
		readFile: func(p string) ([]byte, error) {
			if c, ok := files[p]; ok {
				return []byte(c), nil
			}
			return nil, errors.New("no such file")
		},
	}
}

const coreReceipt = `{"poured_from_bottle":true,"source":{"tap":"homebrew/core"}}`

func brewCellar(prefix string) string { return filepath.Join(prefix, "Cellar") }

func TestBrewDetectViaEnvAndKnownPrefixes(t *testing.T) {
	b := fakeBrew(map[string]string{"HOMEBREW_PREFIX": "/custom"}, nil, nil)
	if b.cellar() != filepath.Join("/custom", "Cellar") {
		t.Fatalf("cellar = %q", b.cellar())
	}

	cellar := brewCellar("/opt/homebrew")
	b = fakeBrew(nil, map[string][]os.DirEntry{cellar: {fakeEntry{name: "jq", dir: true}}}, nil)
	if !b.Detect() {
		t.Fatal("should detect via /opt/homebrew")
	}

	b = fakeBrew(nil, map[string][]os.DirEntry{}, nil)
	if b.Detect() {
		t.Fatal("no cellar anywhere: must not detect")
	}
}

func TestBrewAuditCoreBottle(t *testing.T) {
	cellar := brewCellar("/opt/homebrew")
	b := fakeBrew(nil,
		map[string][]os.DirEntry{
			cellar:                      {fakeEntry{name: "jq", dir: true}},
			filepath.Join(cellar, "jq"): {fakeEntry{name: "1.7.1", dir: true}},
		},
		map[string]string{
			filepath.Join(cellar, "jq", "1.7.1", "INSTALL_RECEIPT.json"): coreReceipt,
		})

	rep := b.Audit(context.Background())

	if len(rep.Packages) != 1 {
		t.Fatalf("packages = %+v", rep.Packages)
	}
	pkg := rep.Packages[0]
	if pkg.Name != "jq" || pkg.Version != "1.7.1" || pkg.Source != "homebrew/core" {
		t.Fatalf("pkg = %+v", pkg)
	}
	if pkg.Integrity != audit.IntegrityNotVerifiable {
		t.Fatalf("integrity = %q — brew must never claim verifiable", pkg.Integrity)
	}
	if len(pkg.Findings) != 0 {
		t.Fatalf("clean core bottle should have no findings: %+v", pkg.Findings)
	}
}

func TestBrewAuditThirdPartyTapAndSourceBuild(t *testing.T) {
	cellar := brewCellar("/opt/homebrew")
	receipt := `{"poured_from_bottle":false,"source":{"tap":"evil/tap"}}`
	b := fakeBrew(nil,
		map[string][]os.DirEntry{
			cellar:                        {fakeEntry{name: "tool", dir: true}},
			filepath.Join(cellar, "tool"): {fakeEntry{name: "2.0", dir: true}},
		},
		map[string]string{
			filepath.Join(cellar, "tool", "2.0", "INSTALL_RECEIPT.json"): receipt,
		})

	pkg := b.Audit(context.Background()).Packages[0]

	codes := findingCodesSys(pkg.Findings)
	if len(codes) != 2 || codes[0] != audit.CodeThirdPartySource || codes[1] != audit.CodeUnpinnedBuild {
		t.Fatalf("findings = %v", codes)
	}
}

func TestBrewAuditMissingReceipt(t *testing.T) {
	cellar := brewCellar("/opt/homebrew")
	b := fakeBrew(nil,
		map[string][]os.DirEntry{
			cellar:                      {fakeEntry{name: "jq", dir: true}},
			filepath.Join(cellar, "jq"): {fakeEntry{name: "1.7.1", dir: true}},
		},
		map[string]string{})

	pkg := b.Audit(context.Background()).Packages[0]

	if pkg.Integrity != audit.IntegrityUnknown {
		t.Fatalf("integrity = %q", pkg.Integrity)
	}
	if len(pkg.Findings) != 1 || pkg.Findings[0].Code != audit.CodeNoIntegrityData {
		t.Fatalf("findings = %+v", pkg.Findings)
	}
}

func findingCodesSys(fs []audit.Finding) []string {
	var codes []string
	for _, f := range fs {
		codes = append(codes, f.Code)
	}
	return codes
}
