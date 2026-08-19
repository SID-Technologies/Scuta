package audit

import (
	"testing"
	"time"
)

func TestStableHashIgnoresGeneratedAt(t *testing.T) {
	a := New("1.2.0")
	b := New("1.2.0")
	b.GeneratedAt = a.GeneratedAt.Add(time.Hour)

	ha, err := a.StableHash()
	if err != nil {
		t.Fatalf("StableHash: %v", err)
	}
	hb, err := b.StableHash()
	if err != nil {
		t.Fatalf("StableHash: %v", err)
	}
	if ha != hb {
		t.Fatal("hash must not depend on generation time")
	}
	if len(ha) != 64 {
		t.Fatalf("hash length = %d", len(ha))
	}
}

func TestStableHashChangesWithFindings(t *testing.T) {
	a := New("1.2.0")
	b := New("1.2.0")
	b.GeneratedAt = a.GeneratedAt
	b.Tools = []Tool{{Name: "pilum", Findings: []Finding{{Severity: SeverityCritical, Code: CodeShadowedBinary, Message: "x"}}}}
	b.Finalize()

	ha, _ := a.StableHash()
	hb, _ := b.StableHash()
	if ha == hb {
		t.Fatal("different findings must hash differently")
	}
}
