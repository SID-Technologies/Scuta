package github

import (
	"errors"
	"net"
	"testing"
)

func TestIsRetryableStatus(t *testing.T) {
	tests := []struct {
		status int
		want   bool
	}{
		{200, false},
		{201, false},
		{301, false},
		{400, false},
		{401, false},
		{403, false},
		{404, false},
		{429, true},
		{500, true},
		{502, true},
		{503, true},
		{504, true},
	}

	for _, tt := range tests {
		got := isRetryableStatus(tt.status)
		if got != tt.want {
			t.Errorf("isRetryableStatus(%d) = %v, want %v", tt.status, got, tt.want)
		}
	}
}

func TestIsRetryableError(t *testing.T) {
	// nil error
	if isRetryableError(nil) {
		t.Error("expected nil error to not be retryable")
	}

	// Regular error — not retryable
	if isRetryableError(errors.New("something went wrong")) {
		t.Error("expected generic error to not be retryable")
	}

	// Connection reset
	if !isRetryableError(errors.New("read tcp: connection reset by peer")) {
		t.Error("expected connection reset to be retryable")
	}

	// EOF
	if !isRetryableError(errors.New("unexpected EOF")) {
		t.Error("expected EOF to be retryable")
	}

	// Timeout error
	if !isRetryableError(&net.DNSError{IsTimeout: true}) {
		t.Error("expected timeout to be retryable")
	}
}
