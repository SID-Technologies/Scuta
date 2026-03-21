package output

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
)

// ProgressBar displays a horizontal progress bar in the terminal.
type ProgressBar struct {
	mu       sync.Mutex
	total    int
	current  int
	width    int
	message  string
	complete bool
	ciMode   bool
}

// NewProgressBar creates a new progress bar.
// width is the number of characters for the bar (default 30 if 0).
func NewProgressBar(total int, width int) *ProgressBar {
	if width == 0 {
		width = 30
	}
	return &ProgressBar{
		total:  total,
		width:  width,
		ciMode: isCI(),
	}
}

// isCI returns true if running in a CI environment.
func isCI() bool {
	ciVars := []string{"CI", "GITHUB_ACTIONS", "GITLAB_CI", "CIRCLECI", "JENKINS_URL", "BUILDKITE"}
	for _, v := range ciVars {
		if os.Getenv(v) != "" {
			return true
		}
	}
	return false
}

// SetMessage sets the current message displayed next to the bar.
func (p *ProgressBar) SetMessage(msg string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.message = msg
}

// Increment increases the progress by 1.
func (p *ProgressBar) Increment() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.current < p.total {
		p.current++
	}
	p.render()
}

// SetProgress sets the current progress value.
func (p *ProgressBar) SetProgress(current int) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.current = current
	if p.current > p.total {
		p.current = p.total
	}
	p.render()
}

// Complete marks the progress bar as complete and renders final state.
func (p *ProgressBar) Complete(message string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.current = p.total
	p.message = message
	p.complete = true
	p.renderFinal()
}

// render updates the progress bar display.
func (p *ProgressBar) render() {
	if p.total == 0 {
		return
	}

	if p.ciMode {
		percent := float64(p.current) / float64(p.total) * 100
		fmt.Printf("  Progress: %d/%d (%.0f%%) %s\n", p.current, p.total, percent, p.message)
		return
	}

	percent := float64(p.current) / float64(p.total) * 100
	filled := int(float64(p.width) * float64(p.current) / float64(p.total))
	empty := p.width - filled

	bar := strings.Repeat("\u2588", filled) + strings.Repeat("\u2591", empty)

	fmt.Printf("\r\033[K  %s%s%s %s%3.0f%%%s %s%s%s",
		Primary, bar, Reset,
		Muted, percent, Reset,
		Muted, p.message, Reset)
}

// renderFinal renders the completed state with a newline.
func (p *ProgressBar) renderFinal() {
	if p.ciMode {
		fmt.Printf("  %s%s%s %s\n", SuccessColor, SymbolSuccess, Reset, p.message)
		return
	}

	bar := strings.Repeat("\u2588", p.width)
	fmt.Printf("\r\033[K  %s%s%s %s100%%%s %s%s%s\n",
		SuccessColor, bar, Reset,
		Muted, Reset,
		SuccessColor, p.message, Reset)
}

// StepProgress is a simpler step-based progress indicator.
type StepProgress struct {
	mu      sync.Mutex
	current int
	total   int
	ciMode  bool
}

// NewStepProgress creates a step progress indicator.
func NewStepProgress(total int) *StepProgress {
	return &StepProgress{
		total:  total,
		ciMode: isCI(),
	}
}

// NextStep advances to the next step and displays it.
func (s *StepProgress) NextStep(stepName string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.current++
	s.render(stepName)
}

// render displays the current step.
func (s *StepProgress) render(stepName string) {
	filled := strings.Repeat("\u25a0", s.current)
	empty := strings.Repeat("\u25a1", s.total-s.current)
	indicator := fmt.Sprintf("[%s%s]", filled, empty)

	if s.ciMode {
		fmt.Printf("  Step %d/%d %s %s\n", s.current, s.total, indicator, stepName)
		return
	}

	fmt.Printf("  %s%d/%d%s %s%s%s %s\n",
		Primary, s.current, s.total, Reset,
		Muted, indicator, Reset,
		stepName)
}

// Complete marks the step progress as done.
func (s *StepProgress) Complete() {
	s.mu.Lock()
	defer s.mu.Unlock()

	indicator := strings.Repeat("\u25a0", s.total)
	if s.ciMode {
		fmt.Printf("  %s Complete [%s]\n", SymbolSuccess, indicator)
		return
	}

	fmt.Printf("  %s%s%s Complete %s[%s]%s\n",
		SuccessColor, SymbolSuccess, Reset,
		Muted, indicator, Reset)
}

// ProgressReader wraps an io.Reader and updates a ProgressBar based on bytes read.
// Use it to show download progress by wrapping an HTTP response body.
type ProgressReader struct {
	reader io.Reader
	bar    *ProgressBar
	read   int64
}

// NewProgressReader creates a ProgressReader that wraps the given reader.
// total is the expected number of bytes (e.g. from Content-Length).
func NewProgressReader(reader io.Reader, total int64) *ProgressReader {
	bar := NewProgressBar(int(total), 30)
	bar.SetMessage("downloading...")
	return &ProgressReader{
		reader: reader,
		bar:    bar,
	}
}

// Read implements io.Reader and updates the progress bar after each read.
func (pr *ProgressReader) Read(p []byte) (int, error) {
	n, err := pr.reader.Read(p)
	if n > 0 {
		pr.read += int64(n)
		pr.bar.SetProgress(int(pr.read))
	}
	return n, err
}

// Complete marks the progress bar as finished with the given message.
func (pr *ProgressReader) Complete(message string) {
	pr.bar.Complete(message)
}

// BytesRead returns the total number of bytes read so far.
func (pr *ProgressReader) BytesRead() int64 {
	return pr.read
}
