package installer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/sid-technologies/scuta/lib/cache"
	"github.com/sid-technologies/scuta/lib/github"
)

func TestSetDownloadCacheToggle(t *testing.T) {
	inst := New(github.NewClient(""), t.TempDir())
	if inst.cache == nil {
		t.Fatal("cache should be enabled by default")
	}

	inst.SetDownloadCache(false)
	if inst.cache != nil {
		t.Fatal("SetDownloadCache(false) should disable the cache")
	}

	inst.SetDownloadCache(true)
	if inst.cache == nil {
		t.Fatal("SetDownloadCache(true) should re-enable the cache")
	}
}

func TestNewWithBinDirEnablesCache(t *testing.T) {
	inst := NewWithBinDir(github.NewClient(""), t.TempDir(), t.TempDir())
	if inst.cache == nil {
		t.Fatal("cache should be enabled by default")
	}
}

// TestFetchVerifiedAssetCacheHit proves a cache hit skips the asset download
// entirely: the asset endpoint returns 500 and counts requests, so any
// download attempt would both fail and be visible.
func TestFetchVerifiedAssetCacheHit(t *testing.T) {
	scutaDir := t.TempDir()

	content := []byte("cached asset bytes")
	sum := sha256.Sum256(content)
	hash := hex.EncodeToString(sum[:])

	var assetRequests atomic.Int64
	mux := http.NewServeMux()
	mux.HandleFunc("/checksums.txt", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, "%s  tool_1.0.0_linux_amd64.tar.gz\n", hash)
	})
	mux.HandleFunc("/asset", func(w http.ResponseWriter, _ *http.Request) {
		assetRequests.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Seed the cache with the verified content.
	src := filepath.Join(scutaDir, "seed")
	if err := os.WriteFile(src, content, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := cache.New(scutaDir).Put(hash, src); err != nil {
		t.Fatal(err)
	}

	inst := New(github.NewClient(""), scutaDir)
	release := &github.Release{
		TagName: "v1.0.0",
		Assets: []github.Asset{
			{Name: "checksums.txt", BrowserDownloadURL: srv.URL + "/checksums.txt"},
			{Name: "tool_1.0.0_linux_amd64.tar.gz", BrowserDownloadURL: srv.URL + "/asset"},
		},
	}

	dest := filepath.Join(t.TempDir(), "tool.tar.gz")
	outcome, err := inst.fetchVerifiedAsset(context.Background(), release,
		"owner/tool", "tool_1.0.0_linux_amd64.tar.gz", srv.URL+"/asset", "tool", dest, false, false)
	if err != nil {
		t.Fatalf("fetchVerifiedAsset: %v", err)
	}
	if !outcome.checksumVerified {
		t.Fatal("expected checksumVerified=true when checksum verification succeeded")
	}

	if n := assetRequests.Load(); n != 0 {
		t.Fatalf("asset endpoint was hit %d time(s); cache hit should skip the download", n)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(content) {
		t.Fatalf("dest content mismatch: %q", got)
	}
}

// TestFetchVerifiedAssetCacheDisabled proves that with the cache disabled the
// download path is taken even when a matching entry exists.
func TestFetchVerifiedAssetCacheDisabled(t *testing.T) {
	scutaDir := t.TempDir()

	content := []byte("some asset bytes")
	sum := sha256.Sum256(content)
	hash := hex.EncodeToString(sum[:])

	mux := http.NewServeMux()
	mux.HandleFunc("/checksums.txt", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, "%s  tool.tar.gz\n", hash)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	src := filepath.Join(scutaDir, "seed")
	if err := os.WriteFile(src, content, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := cache.New(scutaDir).Put(hash, src); err != nil {
		t.Fatal(err)
	}

	inst := New(github.NewClient(""), scutaDir)
	inst.SetDownloadCache(false)

	release := &github.Release{
		TagName: "v1.0.0",
		Assets: []github.Asset{
			{Name: "checksums.txt", BrowserDownloadURL: srv.URL + "/checksums.txt"},
		},
	}

	// The plain-http URL is rejected by DownloadAsset's HTTPS validation,
	// which is exactly the signal that the download path was taken.
	dest := filepath.Join(t.TempDir(), "tool.tar.gz")
	_, err := inst.fetchVerifiedAsset(context.Background(), release,
		"owner/tool", "tool.tar.gz", srv.URL+"/asset", "tool", dest, false, false)
	if err == nil {
		t.Fatal("expected download attempt (and HTTPS rejection) with cache disabled")
	}
}

// TestFetchVerifiedAssetSkipVerifyBypassesCache proves --skip-verify never
// consults the cache: without a trusted checksum there is no safe key.
func TestFetchVerifiedAssetSkipVerifyBypassesCache(t *testing.T) {
	scutaDir := t.TempDir()

	content := []byte("unverified bytes")
	sum := sha256.Sum256(content)
	hash := hex.EncodeToString(sum[:])

	src := filepath.Join(scutaDir, "seed")
	if err := os.WriteFile(src, content, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := cache.New(scutaDir).Put(hash, src); err != nil {
		t.Fatal(err)
	}

	inst := New(github.NewClient(""), scutaDir)
	release := &github.Release{TagName: "v1.0.0"}

	dest := filepath.Join(t.TempDir(), "tool.tar.gz")
	_, err := inst.fetchVerifiedAsset(context.Background(), release,
		"owner/tool", "tool.tar.gz", "http://127.0.0.1:0/asset", "tool", dest, true, false)
	// skipVerify resolves no hash, so it must go straight to the (rejected)
	// download rather than serving the cached entry.
	if err == nil {
		t.Fatal("expected download attempt with --skip-verify")
	}
	if _, statErr := os.Stat(dest); statErr == nil {
		t.Fatal("dest must not be served from cache under --skip-verify")
	}
}
