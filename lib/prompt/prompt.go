// Package prompt provides interactive CLI input helpers.
package prompt

import (
	"bufio"
	"fmt"
	"strconv"
	"strings"

	"github.com/sid-technologies/scuta/lib/errors"
	"github.com/sid-technologies/scuta/lib/output"
)

// Reader wraps a bufio.Reader for interactive prompts.
type Reader struct {
	r *bufio.Reader
}

// NewReader creates a new prompt Reader.
func NewReader(r *bufio.Reader) *Reader {
	return &Reader{r: r}
}

// Ask displays a prompt with an optional default value and returns the user's input.
// If the user enters nothing, the default value is returned.
func (p *Reader) Ask(label, defaultVal string) (string, error) {
	if defaultVal != "" {
		fmt.Printf("  %s [%s]: ", label, defaultVal)
	} else {
		fmt.Printf("  %s: ", label)
	}

	input, err := p.r.ReadString('\n')
	if err != nil {
		return "", errors.Wrap(err, "reading input")
	}

	input = strings.TrimSpace(input)
	if input == "" {
		return defaultVal, nil
	}
	return input, nil
}

// Option represents a selectable choice.
type Option struct {
	Key         string
	Label       string
	Description string
}

// Select displays a numbered list of options and returns the selected key.
// The defaultKey is pre-selected if the user presses enter.
func (p *Reader) Select(header string, options []Option, defaultKey string) (string, error) {
	output.Header(header)

	for i, opt := range options {
		marker := " "
		if opt.Key == defaultKey {
			marker = "*"
		}
		fmt.Printf("  %s %d. %s\n", marker, i+1, opt.Label)
		if opt.Description != "" {
			fmt.Printf("       %s%s%s\n", output.Muted, opt.Description, output.Reset)
		}
	}
	fmt.Println()

	input, err := p.Ask("Select option", "")
	if err != nil {
		return "", err
	}

	if input == "" {
		return defaultKey, nil
	}

	// Allow selection by number
	if num, parseErr := strconv.Atoi(input); parseErr == nil && num > 0 && num <= len(options) {
		return options[num-1].Key, nil
	}

	// Allow selection by key name
	for _, opt := range options {
		if strings.EqualFold(input, opt.Key) {
			return opt.Key, nil
		}
	}

	return "", errors.New("invalid selection: %q", input)
}
