package helper

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestWithSignalCancel_Cleanup(t *testing.T) {
	ctx, cleanup := WithSignalCancel(context.Background())
	defer cleanup()

	// Context should not be canceled yet
	select {
	case <-ctx.Done():
		t.Fatal("context should not be canceled before cleanup")
	default:
		// expected
	}

	// After cleanup, context should be canceled
	cleanup()
	select {
	case <-ctx.Done():
		// expected
	case <-time.After(100 * time.Millisecond):
		t.Fatal("context should be canceled after cleanup")
	}
}

func TestWithSignalCancel_ParentCancel(t *testing.T) {
	parent, parentCancel := context.WithCancel(context.Background())
	ctx, cleanup := WithSignalCancel(parent)
	defer cleanup()

	// Cancel parent
	parentCancel()

	select {
	case <-ctx.Done():
		// expected — child should be canceled when parent is
	case <-time.After(100 * time.Millisecond):
		t.Fatal("child context should be canceled when parent is canceled")
	}
}

func TestEnsureCIEnvironment_Set(t *testing.T) {
	t.Setenv("CI", "true")
	if !EnsureCIEnvironment() {
		t.Error("expected true when CI env var is set")
	}
}

func TestEnsureCIEnvironment_NotSet(t *testing.T) {
	t.Setenv("CI", "")
	os.Unsetenv("CI")
	if EnsureCIEnvironment() {
		t.Error("expected false when CI env var is not set")
	}
}

func TestEnsureCIEnvironment_AnyValue(t *testing.T) {
	t.Setenv("CI", "1")
	if !EnsureCIEnvironment() {
		t.Error("expected true for any non-empty CI value")
	}
}
