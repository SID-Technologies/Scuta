// Package e2e holds live end-to-end tests that exercise the full
// download -> verify -> extract -> install -> run -> uninstall path against
// real GitHub releases.
//
// These tests are guarded by the "e2e" build tag so the default
// `go test ./...` run stays hermetic (no network). Run them explicitly:
//
//	go test -tags=e2e ./test/e2e/...
//
// This file carries no build constraint so the package always has at least
// one buildable Go file, keeping `go build ./...` happy without the tag.
package e2e
