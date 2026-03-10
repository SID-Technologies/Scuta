package policy

import (
	"testing"
)

func TestParse(t *testing.T) {
	data := []byte(`
min_scuta_version: "1.0.0"
tools:
  pilum:
    allowed: ">=1.0.0"
    blocked:
      - "1.2.0"
  torch:
    allowed: "~1.3.0"
`)

	p, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if p.MinScutaVersion != "1.0.0" {
		t.Errorf("MinScutaVersion = %q, want %q", p.MinScutaVersion, "1.0.0")
	}

	if len(p.Tools) != 2 {
		t.Errorf("len(Tools) = %d, want 2", len(p.Tools))
	}

	pilum := p.Tools["pilum"]
	if pilum.Allowed != ">=1.0.0" {
		t.Errorf("pilum.Allowed = %q, want %q", pilum.Allowed, ">=1.0.0")
	}
	if len(pilum.Blocked) != 1 || pilum.Blocked[0] != "1.2.0" {
		t.Errorf("pilum.Blocked = %v, want [1.2.0]", pilum.Blocked)
	}
}

func TestCheckToolVersion_Allowed(t *testing.T) {
	p := &Policy{
		Tools: map[string]ToolPolicy{
			"pilum": {Allowed: ">=1.0.0"},
		},
	}

	v := p.CheckToolVersion("pilum", "1.5.0")
	if v != nil {
		t.Errorf("expected no violation for allowed version, got: %v", v.Message)
	}
}

func TestCheckToolVersion_NotAllowed(t *testing.T) {
	p := &Policy{
		Tools: map[string]ToolPolicy{
			"pilum": {Allowed: ">=2.0.0"},
		},
	}

	v := p.CheckToolVersion("pilum", "1.5.0")
	if v == nil {
		t.Fatal("expected violation for disallowed version, got nil")
	}
	if v.Rule != "not_allowed" {
		t.Errorf("Rule = %q, want %q", v.Rule, "not_allowed")
	}
}

func TestCheckToolVersion_Blocked(t *testing.T) {
	p := &Policy{
		Tools: map[string]ToolPolicy{
			"pilum": {
				Allowed: ">=1.0.0",
				Blocked: []string{"1.2.0"},
			},
		},
	}

	v := p.CheckToolVersion("pilum", "1.2.0")
	if v == nil {
		t.Fatal("expected violation for blocked version, got nil")
	}
	if v.Rule != "blocked" {
		t.Errorf("Rule = %q, want %q", v.Rule, "blocked")
	}
}

func TestCheckToolVersion_UnknownTool(t *testing.T) {
	p := &Policy{
		Tools: map[string]ToolPolicy{
			"pilum": {Allowed: ">=1.0.0"},
		},
	}

	v := p.CheckToolVersion("unknown", "1.0.0")
	if v != nil {
		t.Errorf("expected no violation for unknown tool, got: %v", v.Message)
	}
}

func TestCheckToolVersion_NilPolicy(t *testing.T) {
	var p *Policy
	v := p.CheckToolVersion("pilum", "1.0.0")
	if v != nil {
		t.Errorf("expected no violation for nil policy, got: %v", v.Message)
	}
}

func TestCheckToolVersion_InvalidConstraint(t *testing.T) {
	p := &Policy{
		Tools: map[string]ToolPolicy{
			"pilum": {Allowed: "not-a-constraint"},
		},
	}

	v := p.CheckToolVersion("pilum", "1.0.0")
	if v == nil {
		t.Fatal("expected violation for invalid constraint, got nil")
	}
	if v.Rule != "invalid_constraint" {
		t.Errorf("Rule = %q, want %q", v.Rule, "invalid_constraint")
	}
}

func TestCheckScutaVersion_Meets(t *testing.T) {
	p := &Policy{MinScutaVersion: "1.0.0"}

	v := p.CheckScutaVersion("1.5.0")
	if v != nil {
		t.Errorf("expected no violation, got: %v", v.Message)
	}
}

func TestCheckScutaVersion_Below(t *testing.T) {
	p := &Policy{MinScutaVersion: "2.0.0"}

	v := p.CheckScutaVersion("1.5.0")
	if v == nil {
		t.Fatal("expected violation for old scuta version, got nil")
	}
	if v.Rule != "min_version" {
		t.Errorf("Rule = %q, want %q", v.Rule, "min_version")
	}
}

func TestCheckScutaVersion_NoMinimum(t *testing.T) {
	p := &Policy{}

	v := p.CheckScutaVersion("1.0.0")
	if v != nil {
		t.Errorf("expected no violation when no minimum set, got: %v", v.Message)
	}
}

func TestCheckScutaVersion_NilPolicy(t *testing.T) {
	var p *Policy
	v := p.CheckScutaVersion("1.0.0")
	if v != nil {
		t.Errorf("expected no violation for nil policy, got: %v", v.Message)
	}
}

func TestCheckAll(t *testing.T) {
	p := &Policy{
		Tools: map[string]ToolPolicy{
			"pilum": {Allowed: ">=2.0.0"},
			"torch": {Blocked: []string{"1.0.0"}},
		},
	}

	installed := map[string]string{
		"pilum": "1.5.0",
		"torch": "1.0.0",
		"other": "3.0.0",
	}

	violations := p.CheckAll(installed)
	if len(violations) != 2 {
		t.Fatalf("expected 2 violations, got %d", len(violations))
	}
}

func TestCheckAll_NilPolicy(t *testing.T) {
	var p *Policy
	violations := p.CheckAll(map[string]string{"pilum": "1.0.0"})
	if violations != nil {
		t.Errorf("expected nil violations for nil policy, got %d", len(violations))
	}
}

func TestCheckToolVersion_NoConstraint(t *testing.T) {
	p := &Policy{
		Tools: map[string]ToolPolicy{
			"pilum": {}, // no allowed, no blocked
		},
	}

	v := p.CheckToolVersion("pilum", "1.0.0")
	if v != nil {
		t.Errorf("expected no violation with empty policy, got: %v", v.Message)
	}
}
