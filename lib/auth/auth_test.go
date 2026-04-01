package auth

import (
	"os"
	"testing"
)

func TestResolveTokenWithConfig_EnvVar(t *testing.T) {
	// Set env var
	t.Setenv("SCUTA_GITHUB_TOKEN", "test-token-from-env")

	token := ResolveTokenWithConfig("")
	if token != "test-token-from-env" {
		t.Errorf("expected 'test-token-from-env', got %q", token)
	}
}

func TestResolveTokenWithConfig_EnvVarPriority(t *testing.T) {
	// Env var should take priority over everything
	t.Setenv("SCUTA_GITHUB_TOKEN", "env-token")

	// Even with a scutaDir set, env var should win
	token := ResolveTokenWithConfig("/nonexistent")
	if token != "env-token" {
		t.Errorf("expected env var to take priority, got %q", token)
	}
}

func TestResolveTokenWithConfig_NoToken(t *testing.T) {
	// Clear env var
	t.Setenv("SCUTA_GITHUB_TOKEN", "")
	os.Unsetenv("SCUTA_GITHUB_TOKEN")

	// With no env, no config, no gh CLI → empty string
	token := ResolveTokenWithConfig("/nonexistent-dir-that-wont-have-config")
	// We can't guarantee gh CLI isn't installed, but we can test the function doesn't panic
	_ = token
}

func TestHasToken_WithEnv(t *testing.T) {
	t.Setenv("SCUTA_GITHUB_TOKEN", "has-a-token")
	if !HasToken() {
		t.Error("expected HasToken to return true when env var is set")
	}
}
