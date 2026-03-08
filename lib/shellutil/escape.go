// Package shellutil provides shell-safe string quoting and input validation
// to prevent command injection when constructing shell commands.
package shellutil

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// Quote wraps a string in POSIX-compliant single quotes, escaping any
// embedded single quotes using the '\'' idiom.
func Quote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// toolNameRe matches valid tool names: alphanumeric, hyphens, underscores.
var toolNameRe = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// ValidateToolName returns an error if the name contains characters outside
// [a-zA-Z0-9_-]. Defense-in-depth check before any command construction.
func ValidateToolName(name string) error {
	if name == "" {
		return errors.New("tool name must not be empty")
	}
	if !toolNameRe.MatchString(name) {
		return fmt.Errorf("tool name %q contains invalid characters (allowed: a-z, A-Z, 0-9, '-', '_')", name)
	}
	return nil
}

// SanitizeHeredocValue escapes characters that are dangerous inside an
// unquoted shell heredoc.
func SanitizeHeredocValue(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, "$", `\$`)
	s = strings.ReplaceAll(s, "`", "\\`")
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}
