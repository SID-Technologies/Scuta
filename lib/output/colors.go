package output

// ANSI escape codes - base formatting.
const (
	Reset    = "\033[0m"
	Bold     = "\033[1m"
	DimStyle = "\033[2m"
)

// Raw color codes - use semantic colors below instead.
// Roman-themed palette matching the SID brand.
const (
	rawRed    = "\033[31m"
	rawGold   = "\033[38;5;178m" // Imperial gold (#d4af37)
	rawPurple = "\033[38;5;141m" // Tyrian purple (#9b6dff)
	rawBronze = "\033[38;5;172m" // Roman bronze (#cd7f32)
	rawLaurel = "\033[38;5;108m" // Laurel green (#7a9a6d)
)

// Gradient colors for banner.
const (
	Maroon    = "\033[38;5;52m"  // Deep maroon (darkest)
	DarkRed   = "\033[31m"       // Dark red
	Red       = rawRed           // Standard red
	Crimson   = "\033[38;5;124m" // Rich crimson
	LightRed  = "\033[91m"       // Light red
	BrightRed = "\033[38;5;196m" // Vivid red
	Gold      = rawGold          // Imperial gold
)

// Semantic colors - use these throughout the application.
const (
	// SuccessColor - positive outcomes, completions.
	SuccessColor = rawGold

	// WarningColor - caution, attention needed, in-progress.
	WarningColor = rawBronze

	// ErrorColor - failures, problems.
	ErrorColor = rawRed

	// InfoColor - informational messages.
	InfoColor = rawPurple

	// Muted - de-emphasized text, timestamps, metadata.
	Muted = rawLaurel

	// Primary - main brand/action color.
	Primary = rawPurple

	// Secondary - supporting color.
	Secondary = rawBronze

	// Accent - highlights and emphasis.
	Accent = rawGold
)
