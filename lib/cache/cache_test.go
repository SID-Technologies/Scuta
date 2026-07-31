package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestPutAndGetRoundTrip(t *testing.T) {
	dir := t.TempDir()
	c := New(dir)

	content := []byte("hello scuta cache")
	hash := sha256Hex(content)
	src := filepath.Join(dir, "asset.tar.gz")
	writeFile(t, src, content)

	if err := c.Put(hash, src); err != nil {
		t.Fatalf("Put: %v", err)
	}

	dest := filepath.Join(dir, "restored.tar.gz")
	hit, err := c.Get(hash, dest)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !hit {
		t.Fatal("expected cache hit")
	}

	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(content) {
		t.Fatalf("restored content mismatch: %q", got)
	}
}

func TestGetMissReturnsFalse(t *testing.T) {
	c := New(t.TempDir())
	hash := sha256Hex([]byte("never stored"))

	hit, err := c.Get(hash, filepath.Join(t.TempDir(), "out"))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if hit {
		t.Fatal("expected miss")
	}
}

func TestGetRejectsInvalidKey(t *testing.T) {
	c := New(t.TempDir())

	for _, key := range []string{"", "abc", strings.Repeat("z", 64), "../../etc/passwd"} {
		hit, err := c.Get(key, filepath.Join(t.TempDir(), "out"))
		if err != nil {
			t.Fatalf("Get(%q): %v", key, err)
		}
		if hit {
			t.Fatalf("Get(%q): expected miss for invalid key", key)
		}
	}
}

func TestPutRejectsMismatchedContent(t *testing.T) {
	dir := t.TempDir()
	c := New(dir)

	src := filepath.Join(dir, "asset")
	writeFile(t, src, []byte("actual content"))

	wrongHash := sha256Hex([]byte("different content"))
	if err := c.Put(wrongHash, src); err == nil {
		t.Fatal("expected error caching content that does not match its key")
	}
}

func TestPutRejectsInvalidKey(t *testing.T) {
	dir := t.TempDir()
	c := New(dir)
	src := filepath.Join(dir, "asset")
	writeFile(t, src, []byte("data"))

	if err := c.Put("not-a-hash", src); err == nil {
		t.Fatal("expected error for invalid key")
	}
}

func TestGetSelfHealsCorruptEntry(t *testing.T) {
	dir := t.TempDir()
	c := New(dir)

	content := []byte("original")
	hash := sha256Hex(content)
	src := filepath.Join(dir, "asset")
	writeFile(t, src, content)
	if err := c.Put(hash, src); err != nil {
		t.Fatal(err)
	}

	// Corrupt the stored entry in place.
	entry := filepath.Join(c.Dir(), hash)
	writeFile(t, entry, []byte("tampered"))

	hit, err := c.Get(hash, filepath.Join(dir, "out"))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if hit {
		t.Fatal("corrupt entry must be a miss")
	}
	if _, err := os.Stat(entry); !os.IsNotExist(err) {
		t.Fatal("corrupt entry should have been removed")
	}
}

func TestGetIsCaseInsensitive(t *testing.T) {
	dir := t.TempDir()
	c := New(dir)

	content := []byte("case test")
	hash := sha256Hex(content)
	src := filepath.Join(dir, "asset")
	writeFile(t, src, content)
	if err := c.Put(strings.ToUpper(hash), src); err != nil {
		t.Fatalf("Put uppercase: %v", err)
	}

	hit, err := c.Get(strings.ToUpper(hash), filepath.Join(dir, "out"))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !hit {
		t.Fatal("expected hit with uppercase key")
	}
}

func TestInfoAndClear(t *testing.T) {
	dir := t.TempDir()
	c := New(dir)

	// Empty cache: zero stats, no error.
	stats, err := c.Info()
	if err != nil {
		t.Fatalf("Info on empty cache: %v", err)
	}
	if stats.Entries != 0 || stats.TotalBytes != 0 {
		t.Fatalf("expected empty stats, got %+v", stats)
	}

	for i, content := range [][]byte{[]byte("one"), []byte("two-longer")} {
		src := filepath.Join(dir, "asset")
		writeFile(t, src, content)
		if err := c.Put(sha256Hex(content), src); err != nil {
			t.Fatalf("Put %d: %v", i, err)
		}
	}

	stats, err = c.Info()
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if stats.Entries != 2 {
		t.Fatalf("expected 2 entries, got %d", stats.Entries)
	}
	if stats.TotalBytes != int64(len("one")+len("two-longer")) {
		t.Fatalf("unexpected total bytes: %d", stats.TotalBytes)
	}

	if err := c.Clear(); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	stats, err = c.Info()
	if err != nil {
		t.Fatalf("Info after clear: %v", err)
	}
	if stats.Entries != 0 {
		t.Fatalf("expected empty cache after clear, got %d entries", stats.Entries)
	}
}
