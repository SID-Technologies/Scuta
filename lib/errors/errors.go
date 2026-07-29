// Package errors provides a consistent interface for using errors.
// It also supports slog structured logging attributes; i.e. structured errors.
// It is also a drop-in replacement for the standard library errors package.
package errors

import (
	"fmt"

	pkgerrors "github.com/pkg/errors"
)

// New returns an error that formats as the given text and
// contains the structured (slog) attributes.
//
//nolint:wrapcheck,inamedparam // This function does custom wrapping and errors.
func New(msg string, attrs ...any) error {
	formatted := fmt.Sprintf(msg, attrs...)
	return structured{
		err:   pkgerrors.New(formatted),
		attrs: attrs,
	}
}

// Wrap returns a new error wrapping the provided with additional
// structured fields.
//
//nolint:wrapcheck,inamedparam // This function does custom wrapping and errors.
func Wrap(err error, msg string, attrs ...any) error {
	if err == nil {
		panic("wrap nil error")
	}

	// Support error types that do their own wrapping.
	if wrapper, ok := err.(interface{ Wrap(string, ...any) error }); ok {
		return wrapper.Wrap(msg, attrs...)
	}

	// Format the message with the provided attributes, mirroring New. The
	// attrs double as slog fields, so only Sprintf when some are present to
	// avoid mangling a literal '%' in an attr-less message.
	formatted := msg
	if len(attrs) > 0 {
		formatted = fmt.Sprintf(msg, attrs...)
	}

	var inner structured
	if As(err, &inner) {
		attrs = append(attrs, inner.attrs...) // Append inner attributes
	}

	return structured{
		err:   pkgerrors.Wrap(err, formatted),
		attrs: attrs,
	}
}
