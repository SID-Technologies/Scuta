package helper

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/sid-technologies/scuta/lib/output"
)

// WithSignalCancel wraps a parent context with SIGINT/SIGTERM handling.
// On signal, it prints a warning and cancels the context.
// Call the returned cleanup function in a defer.
func WithSignalCancel(parent context.Context) (context.Context, func()) {
	ctx, cancel := context.WithCancel(parent)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		output.Warning("\nInterrupted, cleaning up...")
		cancel()
	}()

	cleanup := func() {
		signal.Stop(sigChan)
		cancel()
	}

	return ctx, cleanup
}
