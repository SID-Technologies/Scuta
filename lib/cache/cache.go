// Package cache provides a content-addressed download cache keyed by
// verified SHA-256 checksums. Only assets whose checksum was verified
// against a release checksums file are ever stored, so a cache hit is
// as trustworthy as a fresh verified download.
package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"

	"github.com/sid-technologies/scuta/lib/errors"
)

// Cache is a content-addressed store under <scutaDir>/cache/sha256/.
// Entries are named by their lowercase hex SHA-256 digest.
type Cache struct {
	dir string
}

// Stats summarizes the cache contents.
type Stats struct {
	Entries    int
	TotalBytes int64
}

// New returns a Cache rooted at <scutaDir>/cache/sha256.
func New(scutaDir string) *Cache {
	return &Cache{dir: filepath.Join(scutaDir, "cache", "sha256")}
}

// Dir returns the cache directory path.
func (c *Cache) Dir() string {
	return c.dir
}

// validKey reports whether key looks like a lowercase-insensitive hex
// SHA-256 digest. Anything else is refused so cache filenames can never
// contain path separators or other unexpected characters.
func validKey(key string) bool {
	if len(key) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(key)
	return err == nil
}

// normalizeKey lowercases a hex digest so lookups are case-insensitive.
func normalizeKey(key string) string {
	b, err := hex.DecodeString(key)
	if err != nil {
		return key
	}
	return hex.EncodeToString(b)
}

// Get copies the cached entry for hash to destPath and returns true on a
// hit. The entry content is re-hashed before use; a corrupt entry is
// deleted and treated as a miss, never returned.
func (c *Cache) Get(hash, destPath string) (bool, error) {
	if !validKey(hash) {
		return false, nil
	}
	src := filepath.Join(c.dir, normalizeKey(hash))

	if _, err := os.Stat(src); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, errors.Wrap(err, "checking cache entry")
	}

	actual, err := hashFile(src)
	if err != nil {
		return false, errors.Wrap(err, "hashing cache entry")
	}
	if actual != normalizeKey(hash) {
		// Self-heal: drop the corrupt entry and treat as a miss.
		_ = os.Remove(src)
		return false, nil
	}

	if err := copyFile(src, destPath); err != nil {
		return false, errors.Wrap(err, "copying cache entry")
	}
	return true, nil
}

// Put stores srcPath under its verified hash. The write is atomic
// (temp file + rename) so concurrent installs never observe a partial
// entry. Storing is best-effort from the caller's perspective: the file
// content is re-hashed and refused if it does not match hash.
func (c *Cache) Put(hash, srcPath string) error {
	if !validKey(hash) {
		return errors.New("invalid cache key %q: expected hex SHA-256", hash)
	}
	key := normalizeKey(hash)

	actual, err := hashFile(srcPath)
	if err != nil {
		return errors.Wrap(err, "hashing file for cache")
	}
	if actual != key {
		return errors.New("refusing to cache %s: content hash %s does not match key %s", srcPath, actual, key)
	}

	if err := os.MkdirAll(c.dir, 0o700); err != nil {
		return errors.Wrap(err, "creating cache directory")
	}

	dest := filepath.Join(c.dir, key)
	tmp := dest + ".tmp"
	if err := copyFile(srcPath, tmp); err != nil {
		os.Remove(tmp)
		return errors.Wrap(err, "writing cache entry")
	}
	if err := os.Rename(tmp, dest); err != nil {
		os.Remove(tmp)
		return errors.Wrap(err, "committing cache entry")
	}
	return nil
}

// Info returns entry count and total size of the cache.
func (c *Cache) Info() (Stats, error) {
	var stats Stats
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return stats, nil
		}
		return stats, errors.Wrap(err, "reading cache directory")
	}
	for _, e := range entries {
		if e.IsDir() || !validKey(e.Name()) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		stats.Entries++
		stats.TotalBytes += info.Size()
	}
	return stats, nil
}

// Clear removes all cache entries.
func (c *Cache) Clear() error {
	if err := os.RemoveAll(c.dir); err != nil {
		return errors.Wrap(err, "clearing cache")
	}
	return nil
}

// hashFile returns the lowercase hex SHA-256 of a file's content.
func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// copyFile copies src to dst, syncing before close.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}

	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Sync(); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
