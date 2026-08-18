// Package sbom serializes audit reports as Software Bill of Materials
// documents. The CycloneDX output is a pure projection of data the audit
// already collected: no additional inspection happens here.
package sbom

import (
	"crypto/rand"
	"fmt"

	"github.com/sid-technologies/scuta/lib/audit"
)

// CycloneDX 1.5 document constants.
const (
	bomFormat   = "CycloneDX"
	specVersion = "1.5"
)

// Document is a minimal CycloneDX 1.5 BOM. Only the fields scuta populates
// are modeled; the spec treats everything beyond bomFormat/specVersion as
// optional.
type Document struct {
	BOMFormat    string      `json:"bomFormat"`
	SpecVersion  string      `json:"specVersion"`
	SerialNumber string      `json:"serialNumber"`
	Version      int         `json:"version"`
	Metadata     Metadata    `json:"metadata"`
	Components   []Component `json:"components"`
}

// Metadata describes when and by what the BOM was produced.
type Metadata struct {
	Timestamp string     `json:"timestamp"`
	Tools     []MetaTool `json:"tools"`
}

// MetaTool identifies the producing tool.
type MetaTool struct {
	Vendor  string `json:"vendor"`
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Component is one inventoried piece of software.
type Component struct {
	BOMRef     string     `json:"bom-ref"`
	Type       string     `json:"type"`
	Name       string     `json:"name"`
	Version    string     `json:"version,omitempty"`
	PackageURL string     `json:"purl,omitempty"`
	Hashes     []Hash     `json:"hashes,omitempty"`
	Properties []Property `json:"properties,omitempty"`
}

// Hash is a checksum of the component's binary.
type Hash struct {
	Alg     string `json:"alg"`
	Content string `json:"content"`
}

// Property is a namespaced key/value carrying scuta-specific audit state.
type Property struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// FromReport projects an audit report into a CycloneDX document. Managed
// tools come first, then system packages per manager. Managers that only
// summarize their inventory (dpkg) contribute no components.
func FromReport(r *audit.Report) (*Document, error) {
	serial, err := serialNumber()
	if err != nil {
		return nil, err
	}

	doc := &Document{
		BOMFormat:    bomFormat,
		SpecVersion:  specVersion,
		SerialNumber: serial,
		Version:      1,
		Metadata: Metadata{
			Timestamp: r.GeneratedAt.Format("2006-01-02T15:04:05Z07:00"),
			Tools: []MetaTool{
				{Vendor: "sid-technologies", Name: "scuta", Version: r.ScutaVersion},
			},
		},
		Components: []Component{},
	}

	for _, t := range r.Tools {
		doc.Components = append(doc.Components, toolComponent(t))
	}
	if r.System != nil {
		for _, m := range r.System.Managers {
			for _, p := range m.Packages {
				doc.Components = append(doc.Components, packageComponent(m.Name, p))
			}
		}
	}

	return doc, nil
}

// toolComponent maps a scuta-managed tool. The recorded install hash (the
// verified one, not the current on-disk state) is exported; drift and
// shadowing are flagged as properties so consumers see the audit verdict.
func toolComponent(t audit.Tool) Component {
	c := Component{
		Type:    "application",
		Name:    t.Name,
		Version: t.Version,
	}

	if t.Repo != "" {
		c.PackageURL = fmt.Sprintf("pkg:github/%s@%s", t.Repo, t.Version)
	}
	c.BOMRef = bomRef(c.PackageURL, "scuta", t.Name, t.Version)

	if t.Sha256 != "" {
		c.Hashes = []Hash{{Alg: "SHA-256", Content: t.Sha256}}
	}

	c.Properties = append(c.Properties, Property{Name: "scuta:verified", Value: fmt.Sprintf("%t", t.Verified)})
	if t.Drift {
		c.Properties = append(c.Properties, Property{Name: "scuta:drift", Value: "true"})
	}
	if t.Shadowed {
		c.Properties = append(c.Properties, Property{Name: "scuta:shadowed", Value: "true"})
	}

	return c
}

// packageComponent maps one system package. Integrity is exported verbatim:
// not-verifiable stays visible rather than being dropped or upgraded.
func packageComponent(manager string, p audit.SystemPackage) Component {
	c := Component{
		Type:    "application",
		Name:    p.Name,
		Version: p.Version,
	}

	switch manager {
	case "go":
		if p.Source != "" {
			c.PackageURL = fmt.Sprintf("pkg:golang/%s@%s", p.Source, p.Version)
		}
	case "brew":
		c.PackageURL = fmt.Sprintf("pkg:brew/%s@%s", p.Name, p.Version)
	default:
		c.PackageURL = fmt.Sprintf("pkg:generic/%s@%s", p.Name, p.Version)
	}
	c.BOMRef = bomRef(c.PackageURL, manager, p.Name, p.Version)

	c.Properties = append(c.Properties,
		Property{Name: "scuta:manager", Value: manager},
		Property{Name: "scuta:integrity", Value: p.Integrity},
	)

	return c
}

// bomRef returns a document-unique reference: the purl when one exists,
// otherwise an origin-qualified fallback so same-named packages from
// different managers cannot collide.
func bomRef(purl, origin, name, version string) string {
	if purl != "" {
		return purl
	}
	return fmt.Sprintf("%s:%s@%s", origin, name, version)
}

// serialNumber returns a random RFC 4122 version 4 UUID URN, as the
// CycloneDX serialNumber field requires.
func serialNumber() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generating BOM serial number: %w", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // RFC 4122 variant

	return fmt.Sprintf("urn:uuid:%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}
