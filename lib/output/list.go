package output

import (
	"fmt"
	"strings"
)

// TableRow represents a single row in tabular output.
type TableRow struct {
	Columns []string
}

// PrintTable prints rows in aligned columns with a header.
func PrintTable(headers []string, rows []TableRow) {
	if len(headers) == 0 {
		return
	}

	// Calculate column widths
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	for _, row := range rows {
		for i, col := range row.Columns {
			if i < len(widths) && len(col) > widths[i] {
				widths[i] = len(col)
			}
		}
	}

	// Print header
	headerParts := make([]string, len(headers))
	for i, h := range headers {
		headerParts[i] = fmt.Sprintf("%-*s", widths[i], h)
	}
	fmt.Printf("%s%s%s\n", Bold, strings.Join(headerParts, "  "), Reset)

	// Print rows
	for _, row := range rows {
		parts := make([]string, len(headers))
		for i := range headers {
			val := ""
			if i < len(row.Columns) {
				val = row.Columns[i]
			}
			parts[i] = fmt.Sprintf("%-*s", widths[i], val)
		}
		fmt.Println(strings.Join(parts, "  "))
	}
}
