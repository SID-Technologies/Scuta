package suggest

import (
	"testing"
)

func TestSearch_ExactName(t *testing.T) {
	tools := map[string]ToolEntry{
		"fzf":     {Description: "A command-line fuzzy finder"},
		"ripgrep": {Description: "Fast line-oriented search tool"},
		"fd":      {Description: "A simple, fast alternative to find"},
	}

	results := Search("fzf", tools, 10)
	if len(results) == 0 {
		t.Fatal("expected at least one result")
	}
	if results[0].Name != "fzf" {
		t.Fatalf("expected fzf as top result, got %s", results[0].Name)
	}
	if results[0].Score != 100 {
		t.Fatalf("expected score 100 for exact match, got %d", results[0].Score)
	}
}

func TestSearch_SubstringName(t *testing.T) {
	tools := map[string]ToolEntry{
		"golangci-lint": {Description: "Go linter"},
		"goreleaser":    {Description: "Release automation"},
		"gh":            {Description: "GitHub CLI"},
	}

	results := Search("golang", tools, 10)
	if len(results) == 0 {
		t.Fatal("expected at least one result")
	}
	if results[0].Name != "golangci-lint" {
		t.Fatalf("expected golangci-lint as top result, got %s", results[0].Name)
	}
	if results[0].MatchField != "name" {
		t.Fatalf("expected match field 'name', got %s", results[0].MatchField)
	}
}

func TestSearch_FuzzyName(t *testing.T) {
	tools := map[string]ToolEntry{
		"fzf":     {Description: "A command-line fuzzy finder"},
		"ripgrep": {Description: "Fast line-oriented search tool"},
	}

	// "fzg" is distance 1 from "fzf"
	results := Search("fzg", tools, 10)
	if len(results) == 0 {
		t.Fatal("expected at least one fuzzy match result")
	}

	found := false
	for _, r := range results {
		if r.Name == "fzf" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected fzf in fuzzy results")
	}
}

func TestSearch_DescriptionMatch(t *testing.T) {
	tools := map[string]ToolEntry{
		"fzf":     {Description: "A command-line fuzzy finder"},
		"ripgrep": {Description: "Fast line-oriented search tool"},
		"bat":     {Description: "A cat clone with syntax highlighting"},
	}

	// "finder" only appears in fzf's description, not in any name
	results := Search("finder", tools, 10)
	if len(results) == 0 {
		t.Fatal("expected at least one description match")
	}

	found := false
	for _, r := range results {
		if r.Name == "fzf" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected fzf via description match")
	}
}

func TestSearch_MaxResults(t *testing.T) {
	tools := make(map[string]ToolEntry)
	for _, name := range []string{"tool-a", "tool-b", "tool-c", "tool-d", "tool-e"} {
		tools[name] = ToolEntry{Description: "A tool for testing"}
	}

	results := Search("tool", tools, 3)
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
}

func TestSearch_EmptyQuery(t *testing.T) {
	tools := map[string]ToolEntry{
		"fzf": {Description: "fuzzy finder"},
	}

	results := Search("", tools, 10)
	if len(results) != 0 {
		t.Fatal("expected no results for empty query")
	}
}

func TestSearch_NoMatch(t *testing.T) {
	tools := map[string]ToolEntry{
		"fzf": {Description: "fuzzy finder"},
	}

	results := Search("zzzznotfound", tools, 10)
	if len(results) != 0 {
		t.Fatalf("expected no results, got %d", len(results))
	}
}

func TestSearch_CaseInsensitive(t *testing.T) {
	tools := map[string]ToolEntry{
		"FZF":     {Description: "A command-line fuzzy finder"},
		"RipGrep": {Description: "Search tool"},
	}

	results := Search("fzf", tools, 10)
	if len(results) == 0 {
		t.Fatal("expected case-insensitive match")
	}
}
