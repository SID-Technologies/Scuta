package output

import "fmt"

// PrintKV prints a key-value pair with consistent padding.
func PrintKV(key, value string) {
	fmt.Printf("  %s%-14s%s %s\n", Muted, key+":", Reset, value)
}

// FormatBytes returns a human-readable byte size string.
func FormatBytes(bytes int64) string {
	const (
		kb = 1024
		mb = 1024 * kb
		gb = 1024 * mb
	)

	switch {
	case bytes >= gb:
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(gb))
	case bytes >= mb:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(mb))
	case bytes >= kb:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(kb))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

// PrintCheck prints a success/failure check line with a symbol.
func PrintCheck(ok bool, msg string, args ...any) {
	formatted := fmt.Sprintf(msg, args...)
	if ok {
		fmt.Printf("  %s%s%s %s\n", SuccessColor, SymbolSuccess, Reset, formatted)
		return
	}
	fmt.Printf("  %s%s%s %s\n", ErrorColor, SymbolFailure, Reset, formatted)
}

// PrintCheckWarn prints a warning check line with a symbol.
func PrintCheckWarn(msg string, args ...any) {
	formatted := fmt.Sprintf(msg, args...)
	fmt.Printf("  %s%s%s %s\n", WarningColor, SymbolWarning, Reset, formatted)
}
