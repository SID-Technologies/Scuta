package sbom

import (
	"regexp"
	"testing"
	"time"

	"github.com/sid-technologies/scuta/lib/audit"
)

func baseReport() *audit.Report {
	return &audit.Report{
		SchemaVersion: audit.SchemaVersion,
		GeneratedAt:   time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC),
		ScutaVersion:  "1.1.0",
	}
}

func TestFromReportDocumentEnvelope(t *testing.T) {
	doc, err := FromReport(baseReport())
	if err != nil {
		t.Fatalf("FromReport: %v", err)
	}

	if doc.BOMFormat != "CycloneDX" || doc.SpecVersion != "1.5" || doc.Version != 1 {
		t.Fatalf("envelope = %s %s %d", doc.BOMFormat, doc.SpecVersion, doc.Version)
	}
	serialRe := regexp.MustCompile(`^urn:uuid:[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	if !serialRe.MatchString(doc.SerialNumber) {
		t.Fatalf("serialNumber = %q", doc.SerialNumber)
	}
	if doc.Metadata.Timestamp != "2026-08-18T12:00:00Z" {
		t.Fatalf("timestamp = %q", doc.Metadata.Timestamp)
	}
	if len(doc.Metadata.Tools) != 1 || doc.Metadata.Tools[0].Name != "scuta" || doc.Metadata.Tools[0].Version != "1.1.0" {
		t.Fatalf("metadata tools = %+v", doc.Metadata.Tools)
	}
	if doc.Components == nil || len(doc.Components) != 0 {
		t.Fatalf("empty report should serialize an empty (non-nil) components array: %+v", doc.Components)
	}
}

func TestFromReportSerialNumbersAreUnique(t *testing.T) {
	a, err := FromReport(baseReport())
	if err != nil {
		t.Fatalf("FromReport: %v", err)
	}
	b, err := FromReport(baseReport())
	if err != nil {
		t.Fatalf("FromReport: %v", err)
	}
	if a.SerialNumber == b.SerialNumber {
		t.Fatal("serial numbers must differ between documents")
	}
}

func TestToolComponentWithRepoAndHash(t *testing.T) {
	r := baseReport()
	r.Tools = []audit.Tool{{
		Name:     "pilum",
		Version:  "1.2.0",
		Repo:     "sid-technologies/Pilum",
		Sha256:   "abc123",
		Verified: true,
	}}

	doc, err := FromReport(r)
	if err != nil {
		t.Fatalf("FromReport: %v", err)
	}
	if len(doc.Components) != 1 {
		t.Fatalf("components = %d", len(doc.Components))
	}

	c := doc.Components[0]
	if c.PackageURL != "pkg:github/sid-technologies/Pilum@1.2.0" {
		t.Fatalf("purl = %q", c.PackageURL)
	}
	if c.BOMRef != c.PackageURL {
		t.Fatalf("bom-ref should reuse the purl: %q", c.BOMRef)
	}
	if c.Type != "application" {
		t.Fatalf("type = %q", c.Type)
	}
	if len(c.Hashes) != 1 || c.Hashes[0].Alg != "SHA-256" || c.Hashes[0].Content != "abc123" {
		t.Fatalf("hashes = %+v", c.Hashes)
	}
	if !hasProperty(c, "scuta:verified", "true") {
		t.Fatalf("properties = %+v", c.Properties)
	}
	if hasPropertyName(c, "scuta:drift") || hasPropertyName(c, "scuta:shadowed") {
		t.Fatalf("clean tool must not carry drift/shadowed properties: %+v", c.Properties)
	}
}

func TestToolComponentWithoutRepoUsesFallbackRef(t *testing.T) {
	r := baseReport()
	r.Tools = []audit.Tool{{Name: "api-gen", Version: "0.3.0"}}

	doc, err := FromReport(r)
	if err != nil {
		t.Fatalf("FromReport: %v", err)
	}

	c := doc.Components[0]
	if c.PackageURL != "" {
		t.Fatalf("purl = %q", c.PackageURL)
	}
	if c.BOMRef != "scuta:api-gen@0.3.0" {
		t.Fatalf("bom-ref = %q", c.BOMRef)
	}
	if len(c.Hashes) != 0 {
		t.Fatalf("hashes = %+v", c.Hashes)
	}
	if !hasProperty(c, "scuta:verified", "false") {
		t.Fatalf("properties = %+v", c.Properties)
	}
}

func TestToolComponentFlagsDriftAndShadowing(t *testing.T) {
	r := baseReport()
	r.Tools = []audit.Tool{{Name: "pilum", Version: "1.2.0", Drift: true, Shadowed: true}}

	doc, err := FromReport(r)
	if err != nil {
		t.Fatalf("FromReport: %v", err)
	}

	c := doc.Components[0]
	if !hasProperty(c, "scuta:drift", "true") || !hasProperty(c, "scuta:shadowed", "true") {
		t.Fatalf("properties = %+v", c.Properties)
	}
}

func TestSystemPackagePurlsPerManager(t *testing.T) {
	r := baseReport()
	r.System = &audit.System{Managers: []audit.ManagerReport{
		{Name: "go", Detected: true, Packages: []audit.SystemPackage{
			{Name: "gopls", Version: "v0.16.2", Source: "golang.org/x/tools/gopls", Integrity: audit.IntegrityRecorded},
		}},
		{Name: "brew", Detected: true, Packages: []audit.SystemPackage{
			{Name: "jq", Version: "1.7.1", Integrity: audit.IntegrityNotVerifiable},
		}},
		{Name: "mise", Detected: true, Packages: []audit.SystemPackage{
			{Name: "node", Version: "22.13.1", Integrity: audit.IntegrityNotVerifiable},
		}},
		{Name: "dpkg", Detected: false},
	}}

	doc, err := FromReport(r)
	if err != nil {
		t.Fatalf("FromReport: %v", err)
	}
	if len(doc.Components) != 3 {
		t.Fatalf("components = %d", len(doc.Components))
	}

	want := []string{
		"pkg:golang/golang.org/x/tools/gopls@v0.16.2",
		"pkg:brew/jq@1.7.1",
		"pkg:generic/node@22.13.1",
	}
	for i, purl := range want {
		if doc.Components[i].PackageURL != purl {
			t.Fatalf("component %d purl = %q, want %q", i, doc.Components[i].PackageURL, purl)
		}
	}
	if !hasProperty(doc.Components[0], "scuta:integrity", audit.IntegrityRecorded) {
		t.Fatalf("go properties = %+v", doc.Components[0].Properties)
	}
	if !hasProperty(doc.Components[1], "scuta:manager", "brew") {
		t.Fatalf("brew properties = %+v", doc.Components[1].Properties)
	}
	if !hasProperty(doc.Components[2], "scuta:integrity", audit.IntegrityNotVerifiable) {
		t.Fatalf("mise properties = %+v", doc.Components[2].Properties)
	}
}

func TestSystemGoPackageWithoutSourceUsesFallbackRef(t *testing.T) {
	r := baseReport()
	r.System = &audit.System{Managers: []audit.ManagerReport{
		{Name: "go", Detected: true, Packages: []audit.SystemPackage{
			{Name: "mystery", Version: "devel", Integrity: audit.IntegrityUnknown},
		}},
	}}

	doc, err := FromReport(r)
	if err != nil {
		t.Fatalf("FromReport: %v", err)
	}

	c := doc.Components[0]
	if c.PackageURL != "" {
		t.Fatalf("purl = %q", c.PackageURL)
	}
	if c.BOMRef != "go:mystery@devel" {
		t.Fatalf("bom-ref = %q", c.BOMRef)
	}
}

func hasProperty(c Component, name, value string) bool {
	for _, p := range c.Properties {
		if p.Name == name && p.Value == value {
			return true
		}
	}
	return false
}

func hasPropertyName(c Component, name string) bool {
	for _, p := range c.Properties {
		if p.Name == name {
			return true
		}
	}
	return false
}
