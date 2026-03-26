package shellutil

import (
	"testing"
)

func TestQuote_Simple(t *testing.T) {
	got := Quote("hello")
	if got != "'hello'" {
		t.Errorf("expected 'hello', got %q", got)
	}
}

func TestQuote_Empty(t *testing.T) {
	got := Quote("")
	if got != "''" {
		t.Errorf("expected empty quotes, got %q", got)
	}
}

func TestQuote_SingleQuotes(t *testing.T) {
	got := Quote("it's")
	expected := "'it'\\''s'"
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

func TestQuote_Spaces(t *testing.T) {
	got := Quote("hello world")
	if got != "'hello world'" {
		t.Errorf("expected 'hello world', got %q", got)
	}
}

func TestQuote_SpecialChars(t *testing.T) {
	got := Quote("$HOME/bin")
	if got != "'$HOME/bin'" {
		t.Errorf("expected '$HOME/bin', got %q", got)
	}
}

func TestValidateToolName_Valid(t *testing.T) {
	valid := []string{"fzf", "my-tool", "tool_v2", "GoReleaser", "bat123"}
	for _, name := range valid {
		if err := ValidateToolName(name); err != nil {
			t.Errorf("expected %q to be valid, got: %v", name, err)
		}
	}
}

func TestValidateToolName_Empty(t *testing.T) {
	err := ValidateToolName("")
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestValidateToolName_Invalid(t *testing.T) {
	invalid := []string{"my tool", "tool/name", "tool@v2", "$(cmd)", "tool;rm"}
	for _, name := range invalid {
		if err := ValidateToolName(name); err == nil {
			t.Errorf("expected %q to be invalid", name)
		}
	}
}

func TestSanitizeHeredocValue_Backslash(t *testing.T) {
	got := SanitizeHeredocValue(`path\to\file`)
	expected := `path\\to\\file`
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

func TestSanitizeHeredocValue_Dollar(t *testing.T) {
	got := SanitizeHeredocValue("$HOME")
	expected := `\$HOME`
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

func TestSanitizeHeredocValue_Backtick(t *testing.T) {
	got := SanitizeHeredocValue("`whoami`")
	expected := "\\`whoami\\`"
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

func TestSanitizeHeredocValue_DoubleQuote(t *testing.T) {
	got := SanitizeHeredocValue(`say "hello"`)
	expected := `say \"hello\"`
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

func TestSanitizeHeredocValue_Combined(t *testing.T) {
	got := SanitizeHeredocValue(`$HOME\bin "test" ` + "`cmd`")
	if got == `$HOME\bin "test" `+"`cmd`" {
		t.Error("expected all dangerous chars to be escaped")
	}
}
