package prompt

import (
	"bufio"
	"strings"
	"testing"
)

func newTestReader(input string) *Reader {
	return NewReader(bufio.NewReader(strings.NewReader(input)))
}

func TestAsk_UserInput(t *testing.T) {
	r := newTestReader("my-answer\n")
	got, err := r.Ask("Name", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "my-answer" {
		t.Errorf("expected 'my-answer', got %q", got)
	}
}

func TestAsk_Default(t *testing.T) {
	r := newTestReader("\n")
	got, err := r.Ask("Name", "default-val")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "default-val" {
		t.Errorf("expected 'default-val', got %q", got)
	}
}

func TestAsk_TrimWhitespace(t *testing.T) {
	r := newTestReader("  trimmed  \n")
	got, err := r.Ask("Name", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "trimmed" {
		t.Errorf("expected 'trimmed', got %q", got)
	}
}

func TestAsk_OverrideDefault(t *testing.T) {
	r := newTestReader("override\n")
	got, err := r.Ask("Name", "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "override" {
		t.Errorf("expected 'override', got %q", got)
	}
}

func TestAsk_EOF(t *testing.T) {
	r := newTestReader("")
	_, err := r.Ask("Name", "")
	if err == nil {
		t.Fatal("expected error on EOF")
	}
}

func TestSelect_ByNumber(t *testing.T) {
	r := newTestReader("2\n")
	options := []Option{
		{Key: "a", Label: "Option A"},
		{Key: "b", Label: "Option B"},
		{Key: "c", Label: "Option C"},
	}
	got, err := r.Select("Pick one", options, "a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "b" {
		t.Errorf("expected 'b', got %q", got)
	}
}

func TestSelect_ByKey(t *testing.T) {
	r := newTestReader("c\n")
	options := []Option{
		{Key: "a", Label: "Option A"},
		{Key: "b", Label: "Option B"},
		{Key: "c", Label: "Option C"},
	}
	got, err := r.Select("Pick one", options, "a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "c" {
		t.Errorf("expected 'c', got %q", got)
	}
}

func TestSelect_Default(t *testing.T) {
	r := newTestReader("\n")
	options := []Option{
		{Key: "a", Label: "Option A"},
		{Key: "b", Label: "Option B"},
	}
	got, err := r.Select("Pick one", options, "b")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "b" {
		t.Errorf("expected default 'b', got %q", got)
	}
}

func TestSelect_CaseInsensitiveKey(t *testing.T) {
	r := newTestReader("B\n")
	options := []Option{
		{Key: "a", Label: "Option A"},
		{Key: "b", Label: "Option B"},
	}
	got, err := r.Select("Pick one", options, "a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "b" {
		t.Errorf("expected 'b', got %q", got)
	}
}

func TestSelect_InvalidSelection(t *testing.T) {
	r := newTestReader("xyz\n")
	options := []Option{
		{Key: "a", Label: "Option A"},
		{Key: "b", Label: "Option B"},
	}
	_, err := r.Select("Pick one", options, "a")
	if err == nil {
		t.Fatal("expected error for invalid selection")
	}
}

func TestSelect_OutOfRangeNumber(t *testing.T) {
	r := newTestReader("99\n")
	options := []Option{
		{Key: "a", Label: "Option A"},
		{Key: "b", Label: "Option B"},
	}
	_, err := r.Select("Pick one", options, "a")
	if err == nil {
		t.Fatal("expected error for out-of-range number")
	}
}
