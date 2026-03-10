package output

import (
	"fmt"
	"os"
	"strings"
)

var bannerLines = []string{
	` .oooooo..o   .oooooo.   ooooo     ooo ooooooooooooo       .o.`,
	`d8P'    'Y8  d8P'  'Y8b  '888'     '8' 8'   888   '8      .888.`,
	`Y88bo.      888           888       8       888          .8"888.`,
	` '"Y8888o.  888           888       8       888         .8' '888.`,
	`     '"Y88b 888           888       8       888        .88ooo8888.`,
	`oo     .d8P '88b    ooo   '88.    .8'       888       .8'     '888.`,
	`8""88888P'   'Y8bood8P'     'YbodP'        o888o     o88o     o8888o`,
}

var shieldLines = []string{
	`.━━━━━━━━.`,
	`┃⠱⣄⠀⢸⡇⠀⣠⠎┃`,
	`┃⠀⠈⢦⣸⣇⡴⠁⠀┃`,
	`┃⠶⠶⠶⣿⣿⠶⠶⠶┃`,
	`┃⠀⢀⠞⢹⡏⠳⡀⠀┃`,
	`┃⡰⠋⠀⢸⡇⠀⠙⢆┃`,
	`'━━━━━━━━'`,
}

const bannerWidth = 68 // pad all banner lines to this width

func colorShieldLine(line string) string {
	var colored strings.Builder
	for _, r := range line {
		switch r {
		case '.', '-', '|', '\'', '━', '┃':
			colored.WriteString(Crimson)
			colored.WriteRune(r)
			colored.WriteString(Reset)
		case ' ':
			colored.WriteRune(r)
		default:
			colored.WriteString(Gold)
			colored.WriteRune(r)
			colored.WriteString(Reset)
		}
	}
	return colored.String()
}

func buildCombined(colorize bool) string {
	bannerColors := []string{
		Maroon,
		DarkRed,
		Red,
		Crimson,
		LightRed,
		BrightRed,
		BrightRed,
	}

	gap := "  "
	var lines []string

	for i := range bannerLines {
		padded := bannerLines[i] + strings.Repeat(" ", bannerWidth-len(bannerLines[i]))

		if colorize {
			coloredBanner := bannerColors[i] + padded + Reset
			lines = append(lines, coloredBanner+gap+colorShieldLine(shieldLines[i]))
		} else {
			lines = append(lines, padded+gap+shieldLines[i])
		}
	}

	return strings.Join(lines, "\n")
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
	combined := buildCombined(supportsColor())

	if !supportsColor() {
		return fmt.Sprintf("\n%s\n\nVersion: %s\n", combined, version)
	}

	return fmt.Sprintf("\n%s\n\nVersion: %s%s%s\n", combined, Gold, version, Reset)
}
