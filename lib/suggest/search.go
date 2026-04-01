package suggest

import (
	"sort"
	"strings"
)

// SearchResult represents a tool that matched a search query.
type SearchResult struct {
	Name       string
	Score      int
	MatchField string // "name" or "description"
}

// Search performs fuzzy search across tool names and descriptions.
// Scoring: exact substring in name (100) > fuzzy match in name (50-90) > substring in description (25).
// Results are sorted by score (highest first), then alphabetically.
func Search(query string, tools map[string]ToolEntry, maxResults int) []SearchResult {
	if query == "" || len(tools) == 0 || maxResults <= 0 {
		return nil
	}

	query = strings.ToLower(query)
	var results []SearchResult

	for name, entry := range tools {
		lowerName := strings.ToLower(name)
		lowerDesc := strings.ToLower(entry.Description)

		var bestScore int
		var matchField string

		// Exact name match
		if lowerName == query {
			bestScore = 100
			matchField = "name"
		} else if strings.Contains(lowerName, query) {
			// Substring match in name
			bestScore = 80
			matchField = "name"
		} else {
			// Fuzzy match in name (Levenshtein)
			dist := LevenshteinDistance(query, name)
			if dist <= MaxDistance {
				bestScore = 90 - (dist * 10) // dist 1 = 80, dist 2 = 70, dist 3 = 60
				matchField = "name"
			}
		}

		// Substring match in description
		if strings.Contains(lowerDesc, query) {
			descScore := 25
			if descScore > bestScore {
				bestScore = descScore
				matchField = "description"
			}
		}

		if bestScore > 0 {
			results = append(results, SearchResult{
				Name:       name,
				Score:      bestScore,
				MatchField: matchField,
			})
		}
	}

	// Sort by score descending, then alphabetically
	sort.Slice(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		return results[i].Name < results[j].Name
	})

	if len(results) > maxResults {
		results = results[:maxResults]
	}

	return results
}

// ToolEntry holds the minimal info needed for search.
type ToolEntry struct {
	Description string
}
