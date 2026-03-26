package exitcodes

import (
	"errors"
	"fmt"
	"testing"
)

func TestCodeFrom_Nil(t *testing.T) {
	if code := CodeFrom(nil); code != Success {
		t.Errorf("expected Success (0), got %d", code)
	}
}

func TestCodeFrom_DirectError(t *testing.T) {
	err := NewError(Network, "connection failed")
	if code := CodeFrom(err); code != Network {
		t.Errorf("expected Network (%d), got %d", Network, code)
	}
}

func TestCodeFrom_WrappedError(t *testing.T) {
	inner := NewError(Install, "checksum mismatch")
	wrapped := fmt.Errorf("install failed: %w", inner)
	if code := CodeFrom(wrapped); code != Install {
		t.Errorf("expected Install (%d), got %d", Install, code)
	}
}

func TestCodeFrom_NonExitCodeError(t *testing.T) {
	err := errors.New("generic error")
	if code := CodeFrom(err); code != General {
		t.Errorf("expected General (%d), got %d", General, code)
	}
}

func TestWithCode(t *testing.T) {
	inner := errors.New("something broke")
	err := WithCode(IO, inner)

	if err.Code != IO {
		t.Errorf("expected code %d, got %d", IO, err.Code)
	}
	if err.Unwrap() != inner {
		t.Error("expected inner error to be preserved")
	}
	if err.Error() != "something broke" {
		t.Errorf("expected inner error message, got %q", err.Error())
	}
}

func TestNewError(t *testing.T) {
	err := NewError(Config, "bad yaml")
	if err.Code != Config {
		t.Errorf("expected code %d, got %d", Config, err.Code)
	}
	if err.Message != "bad yaml" {
		t.Errorf("expected message 'bad yaml', got %q", err.Message)
	}
	if err.Error() != "bad yaml" {
		t.Errorf("expected Error() to return message, got %q", err.Error())
	}
}

func TestError_UnknownFallback(t *testing.T) {
	err := &Error{}
	if err.Error() != "unknown error" {
		t.Errorf("expected 'unknown error', got %q", err.Error())
	}
}

func TestError_MessageOverErr(t *testing.T) {
	err := &Error{
		Message: "custom message",
		Err:     errors.New("inner"),
	}
	if err.Error() != "custom message" {
		t.Errorf("expected Message to take precedence, got %q", err.Error())
	}
}
