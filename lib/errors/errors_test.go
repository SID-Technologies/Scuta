package errors

import (
	stderrors "errors"
	"strings"
	"testing"
)

func TestNew(t *testing.T) {
	err := New("something failed")
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	if err.Error() != "something failed" {
		t.Errorf("expected 'something failed', got %q", err.Error())
	}
}

func TestNew_Formatted(t *testing.T) {
	err := New("failed for %s with code %d", "reason", 42)
	if !strings.Contains(err.Error(), "failed for reason with code 42") {
		t.Errorf("expected formatted message, got %q", err.Error())
	}
}

func TestWrap(t *testing.T) {
	inner := New("inner error")
	wrapped := Wrap(inner, "outer context")
	if wrapped == nil {
		t.Fatal("expected non-nil error")
	}
	if !strings.Contains(wrapped.Error(), "outer context") {
		t.Errorf("expected outer context in message, got %q", wrapped.Error())
	}
	if !strings.Contains(wrapped.Error(), "inner error") {
		t.Errorf("expected inner error in message, got %q", wrapped.Error())
	}
}

func TestWrap_NilPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on nil error")
		}
	}()
	_ = Wrap(nil, "should panic")
}

func TestWrap_WithAttrs(t *testing.T) {
	inner := stderrors.New("base error")
	wrapped := Wrap(inner, "context", "attr1")
	if !strings.Contains(wrapped.Error(), "context") {
		t.Errorf("expected context in wrap message, got %q", wrapped.Error())
	}
	if !strings.Contains(wrapped.Error(), "base error") {
		t.Errorf("expected base error in wrap message, got %q", wrapped.Error())
	}
}

func TestStructured_Attrs(t *testing.T) {
	err := New("test error %s", "val1")
	s, ok := err.(structured)
	if !ok {
		t.Fatal("expected structured error type")
	}
	attrs := s.Attrs()
	if len(attrs) == 0 {
		t.Fatal("expected attributes")
	}
}

func TestWrap_AccumulatesAttrs(t *testing.T) {
	inner := New("inner %s", "inner_key")
	outer := Wrap(inner, "outer %s", "outer_key")
	s, ok := outer.(structured)
	if !ok {
		t.Fatal("expected structured error type")
	}
	attrs := s.Attrs()
	if len(attrs) < 2 {
		t.Errorf("expected accumulated attributes, got %d", len(attrs))
	}
}

func TestIs(t *testing.T) {
	base := stderrors.New("base")
	if !Is(base, base) {
		t.Error("expected Is to match same error")
	}

	other := stderrors.New("other")
	if Is(base, other) {
		t.Error("expected Is to not match different errors")
	}
}

func TestAs(t *testing.T) {
	err := New("test error")
	var s structured
	if !As(err, &s) {
		t.Error("expected As to match structured error")
	}
}

func TestUnwrap(t *testing.T) {
	inner := stderrors.New("inner")
	wrapped := Wrap(inner, "context")
	unwrapped := Unwrap(wrapped)
	if unwrapped == nil {
		t.Fatal("expected non-nil unwrapped error")
	}
}
