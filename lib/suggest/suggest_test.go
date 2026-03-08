package suggest

import (
	"testing"
)

func TestLevenshteinDistance(t *testing.T) {
	tests := []struct {
		name     string
		a        string
		b        string
		expected int
	}{
		{"identical strings", "hello", "hello", 0},
		{"empty strings", "", "", 0},
		{"one empty", "hello", "", 5},
		{"other empty", "", "world", 5},
		{"single substitution", "cat", "bat", 1},
		{"single insertion", "cat", "cats", 1},
		{"single deletion", "cats", "cat", 1},
		{"two substitutions", "cat", "dog", 3},
		{"case insensitive", "Hello", "hello", 0},
		{"transposition", "pilum", "pilmu", 2},
		{"common typo - missing char", "api-gen", "api-ge", 1},
		{"common typo - extra char", "pilum", "pilumm", 1},
		{"common typo - wrong char", "pilum", "pilam", 1},
		{"completely different", "apple", "orange", 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := LevenshteinDistance(tt.a, tt.b)
			if result != tt.expected {
				t.Errorf("LevenshteinDistance(%q, %q) = %d, want %d", tt.a, tt.b, result, tt.expected)
			}
		})
	}
}

func TestFindClosest(t *testing.T) {
	candidates := []string{"api-gen", "pilum", "mcp-gen"}

	tests := []struct {
		name       string
		input      string
		maxResults int
		wantLen    int
	}{
		{
			name:       "single typo - pilm instead of pilum",
			input:      "pilm",
			maxResults: 1,
			wantLen:    1,
		},
		{
			name:       "typo - api-gn instead of api-gen",
			input:      "api-gn",
			maxResults: 1,
			wantLen:    1,
		},
		{
			name:       "no close match",
			input:      "docker",
			maxResults: 3,
			wantLen:    0,
		},
		{
			name:       "empty candidates",
			input:      "pilum",
			maxResults: 3,
			wantLen:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testCandidates := candidates
			if tt.name == "empty candidates" {
				testCandidates = nil
			}
			result := FindClosest(tt.input, testCandidates, tt.maxResults)
			if len(result) != tt.wantLen {
				t.Errorf("FindClosest(%q) returned %d results, want %d", tt.input, len(result), tt.wantLen)
			}
		})
	}
}

func TestFormatSuggestion(t *testing.T) {
	candidates := []string{"api-gen", "pilum", "mcp-gen"}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "single suggestion - pilum typo",
			input:    "pilm",
			expected: "did you mean 'pilum'?",
		},
		{
			name:     "no suggestion for distant string",
			input:    "docker",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatSuggestion(tt.input, candidates)
			if result != tt.expected {
				t.Errorf("FormatSuggestion(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestHasCloseMatch(t *testing.T) {
	candidates := []string{"api-gen", "pilum", "mcp-gen"}

	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"has close match - typo", "pilm", true},
		{"no close match", "docker", false},
		{"exact match counts as close", "pilum", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HasCloseMatch(tt.input, candidates)
			if result != tt.expected {
				t.Errorf("HasCloseMatch(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}
