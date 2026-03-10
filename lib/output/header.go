package output

import (
	"fmt"
	"os"
	"strings"
)

const banner = `
  ____    ____   _   _   _____      _
 / ___|  / ___| | | | | |_   _|    / \
 \___ \ | |     | | | |   | |     / _ \
  ___) || |___  | |_| |   | |    / ___ \
 |____/  \____|  \___/    |_|   /_/   \_\

`

func colorGradient(text string) string {
	lines := strings.Split(text, "\n")
	coloredLines := make([]string, len(lines))

	colors := []string{
		rawPurple, // Tyrian purple at top
		rawPurple,
		rawBronze, // Roman bronze
		rawBronze,
		rawGold, // Imperial gold at bottom
		rawGold,
	}

	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			coloredLines[i] = line
			continue
		}

		colorIndex := (i * len(colors)) / len(lines)
		if colorIndex >= len(colors) {
			colorIndex = len(colors) - 1
		}
		color := colors[colorIndex]
		coloredLines[i] = color + line + Reset
	}

	return strings.Join(coloredLines, "\n")
}

func supportsColor() bool {
	_, exists := os.LookupEnv("TERM")
	result := exists && os.Getenv("TERM") != "dumb"

	if _, exists := os.LookupEnv("NO_COLOR"); exists {
		result = false
	}

	return result
}

// PrintBanner returns the formatted banner with version info.
func PrintBanner(version string) string {
	if !supportsColor() {
		return fmt.Sprintf("%s\nVersion: %s\n", banner, version)
	}

	coloredBanner := colorGradient(banner)
	return fmt.Sprintf("%s\nVersion: %s%s%s\n", coloredBanner, rawGold, version, Reset)
}
