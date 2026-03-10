// Package installer handles downloading, verifying, and installing tool binaries.
package installer

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/sid-technologies/scuta/lib/errors"
	"github.com/sid-technologies/scuta/lib/github"
	"github.com/sid-technologies/scuta/lib/output"
)

// Installer manages downloading and installing tool binaries.
type Installer struct {
	github   *github.Client
	scutaDir string
	binDir   string
}

// InstallResult holds the outcome of an install operation.
type InstallResult struct {
	Version    string
	BinaryPath string
}

// New creates an Installer.
func New(ghClient *github.Client, scutaDir string) *Installer {
	return &Installer{
		github:   ghClient,
		scutaDir: scutaDir,
		binDir:   filepath.Join(scutaDir, "bin"),
	}
}

// Install downloads and installs a tool binary from GitHub Releases.
func (inst *Installer) Install(ctx context.Context, toolName string, repo string, targetVersion string, force bool, skipVerify bool) (*InstallResult, error) {
	if err := validateToolName(toolName); err != nil {
		return nil, err
	}

	// Check for cancellation before starting
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// Get the release
	var release *github.Release
	var err error

	if targetVersion != "" {
		release, err = inst.github.GetRelease(ctx, repo, targetVersion)
	} else {
		release, err = inst.github.GetLatestRelease(ctx, repo)
	}
	if err != nil {
		return nil, errors.Wrap(err, "fetching release for %s", toolName)
	}

	version := github.NormalizeVersion(release.TagName)
	binaryPath := filepath.Join(inst.binDir, toolName)

	// Check if already installed at this version
	if !force {
		if _, err := os.Stat(binaryPath); err == nil {
			output.Debugf("%s already exists at %s", toolName, binaryPath)
		}
	}

	// Find matching asset
	asset, err := github.FindAsset(release.Assets, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return nil, errors.Wrap(err, "finding asset for %s", toolName)
	}

	output.Debugf("Found asset: %s (%d bytes)", asset.Name, asset.Size)

	// Download to temp directory
	tmpDir, err := os.MkdirTemp("", "scuta-install-*")
	if err != nil {
		return nil, errors.Wrap(err, "creating temp directory")
	}
	defer os.RemoveAll(tmpDir)

	archivePath := filepath.Join(tmpDir, asset.Name)
	if err := inst.github.DownloadAsset(ctx, asset.BrowserDownloadURL, archivePath); err != nil {
		return nil, errors.Wrap(err, "downloading %s", asset.Name)
	}

	// Checksum verification (fail-closed: any failure is an error unless --skip-verify)
	if skipVerify {
		output.Warning("Skipping checksum verification (--skip-verify)")
	} else {
		checksums, csErr := inst.github.DownloadChecksums(ctx, release)
		if csErr != nil {
			return nil, errors.Wrap(csErr, "checksum verification failed for %s: could not download checksums", toolName)
		}
		if checksums == nil {
			return nil, errors.New("checksum verification failed for %s: no checksums file in release (use --skip-verify to override)", toolName)
		}
		expectedHash, ok := checksums[asset.Name]
		if !ok {
			return nil, errors.New("checksum verification failed for %s: no entry for %s in checksums file (use --skip-verify to override)", toolName, asset.Name)
		}
		if err := VerifyChecksum(archivePath, expectedHash); err != nil {
			return nil, errors.Wrap(err, "checksum verification failed for %s", toolName)
		}
		output.Debugf("Checksum verified for %s", asset.Name)
	}

	// Check for cancellation before extraction
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// Extract archive
	extractDir := filepath.Join(tmpDir, "extracted")
	if err := os.MkdirAll(extractDir, 0o755); err != nil {
		return nil, errors.Wrap(err, "creating extract directory")
	}

	if strings.HasSuffix(strings.ToLower(asset.Name), ".tar.gz") || strings.HasSuffix(strings.ToLower(asset.Name), ".tgz") {
		if err := extractTarGz(archivePath, extractDir); err != nil {
			return nil, errors.Wrap(err, "extracting tar.gz")
		}
	} else if strings.HasSuffix(strings.ToLower(asset.Name), ".zip") {
		if err := extractZip(archivePath, extractDir); err != nil {
			return nil, errors.Wrap(err, "extracting zip")
		}
	} else {
		return nil, errors.New("unsupported archive format: %s", asset.Name)
	}

	// Find the binary in extracted contents
	binarySrc, err := findBinary(extractDir, toolName)
	if err != nil {
		return nil, errors.Wrap(err, "finding binary in archive")
	}

	// Check for cancellation before installing binary
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// Ensure bin directory exists
	if err := os.MkdirAll(inst.binDir, 0o755); err != nil {
		return nil, errors.Wrap(err, "creating bin directory")
	}

	// Atomic install: copy to temp, then rename
	tempPath := binaryPath + ".tmp"
	if err := copyFile(binarySrc, tempPath); err != nil {
		os.Remove(tempPath)
		return nil, errors.Wrap(err, "installing binary")
	}

	if err := os.Chmod(tempPath, 0o755); err != nil {
		os.Remove(tempPath)
		return nil, errors.Wrap(err, "setting binary permissions")
	}

	if err := os.Rename(tempPath, binaryPath); err != nil {
		os.Remove(tempPath)
		return nil, errors.Wrap(err, "atomic rename of binary")
	}

	output.Debugf("Installed %s %s to %s", toolName, version, binaryPath)

	return &InstallResult{
		Version:    version,
		BinaryPath: binaryPath,
	}, nil
}

// InstallFromArchive installs a tool from a local archive file (offline/air-gapped install).
func (inst *Installer) InstallFromArchive(toolName string, archivePath string) (*InstallResult, error) {
	// Validate the archive exists
	if _, err := os.Stat(archivePath); err != nil {
		return nil, errors.Wrap(err, "archive file not found")
	}

	// Extract archive to temp directory
	tmpDir, err := os.MkdirTemp("", "scuta-offline-*")
	if err != nil {
		return nil, errors.Wrap(err, "creating temp directory")
	}
	defer os.RemoveAll(tmpDir)

	extractDir := filepath.Join(tmpDir, "extracted")
	if err := os.MkdirAll(extractDir, 0o755); err != nil {
		return nil, errors.Wrap(err, "creating extract directory")
	}

	lowerName := strings.ToLower(archivePath)
	if strings.HasSuffix(lowerName, ".tar.gz") || strings.HasSuffix(lowerName, ".tgz") {
		if err := extractTarGz(archivePath, extractDir); err != nil {
			return nil, errors.Wrap(err, "extracting tar.gz")
		}
	} else if strings.HasSuffix(lowerName, ".zip") {
		if err := extractZip(archivePath, extractDir); err != nil {
			return nil, errors.Wrap(err, "extracting zip")
		}
	} else {
		return nil, errors.New("unsupported archive format: %s (expected .tar.gz, .tgz, or .zip)", filepath.Base(archivePath))
	}

	// Find the binary in extracted contents
	binarySrc, err := findBinary(extractDir, toolName)
	if err != nil {
		return nil, errors.Wrap(err, "finding binary in archive")
	}

	// Ensure bin directory exists
	if err := os.MkdirAll(inst.binDir, 0o755); err != nil {
		return nil, errors.Wrap(err, "creating bin directory")
	}

	// Atomic install: copy to temp, then rename
	binaryPath := filepath.Join(inst.binDir, toolName)
	tempPath := binaryPath + ".tmp"
	if err := copyFile(binarySrc, tempPath); err != nil {
		os.Remove(tempPath)
		return nil, errors.Wrap(err, "installing binary")
	}

	if err := os.Chmod(tempPath, 0o755); err != nil {
		os.Remove(tempPath)
		return nil, errors.Wrap(err, "setting binary permissions")
	}

	if err := os.Rename(tempPath, binaryPath); err != nil {
		os.Remove(tempPath)
		return nil, errors.Wrap(err, "atomic rename of binary")
	}

	// Try to parse version from filename, fallback to "local"
	version := parseVersionFromFilename(filepath.Base(archivePath))

	output.Debugf("Installed %s %s from archive to %s", toolName, version, binaryPath)

	return &InstallResult{
		Version:    version,
		BinaryPath: binaryPath,
	}, nil
}

// parseVersionFromFilename tries to extract a semver-like version from a filename.
// Returns "local" if no version pattern is found.
func parseVersionFromFilename(filename string) string {
	// Remove known extensions
	name := filename
	for _, ext := range []string{".tar.gz", ".tgz", ".zip"} {
		name = strings.TrimSuffix(name, ext)
	}

	// Look for version-like patterns (v1.2.3 or 1.2.3)
	parts := strings.Split(name, "_")
	for _, part := range parts {
		cleaned := strings.TrimPrefix(part, "v")
		// Simple check: contains dots and starts with a digit
		if len(cleaned) > 0 && cleaned[0] >= '0' && cleaned[0] <= '9' && strings.Contains(cleaned, ".") {
			return cleaned
		}
	}

	// Also try with dash separator
	parts = strings.Split(name, "-")
	for _, part := range parts {
		cleaned := strings.TrimPrefix(part, "v")
		if len(cleaned) > 0 && cleaned[0] >= '0' && cleaned[0] <= '9' && strings.Contains(cleaned, ".") {
			return cleaned
		}
	}

	return "local"
}

// Uninstall removes a tool binary from the bin directory.
func (inst *Installer) Uninstall(toolName string) error {
	if err := validateToolName(toolName); err != nil {
		return err
	}

	binaryPath := filepath.Join(inst.binDir, toolName)

	if err := os.Remove(binaryPath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return errors.Wrap(err, "removing binary %s", toolName)
	}

	output.Debugf("Removed %s from %s", toolName, binaryPath)
	return nil
}

// maxFileSize is the maximum allowed size for a single extracted file (100 MB).
const maxFileSize = 100 * 1024 * 1024

// extractTarGz extracts a .tar.gz archive to the destination directory.
func extractTarGz(src string, dest string) error {
	f, err := os.Open(src)
	if err != nil {
		return errors.Wrap(err, "opening archive")
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return errors.Wrap(err, "creating gzip reader")
	}
	defer gz.Close()

	tr := tar.NewReader(gz)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return errors.Wrap(err, "reading tar entry")
		}

		// Reject symlinks and hard links
		if header.Typeflag == tar.TypeSymlink || header.Typeflag == tar.TypeLink {
			return errors.New("archive contains a symlink or hard link: %s (rejected for security)", header.Name)
		}

		// Prevent path traversal
		if !isSafePath(dest, header.Name) {
			return errors.New("archive contains path traversal: %s", header.Name)
		}

		target := filepath.Join(dest, filepath.Clean(header.Name))

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return errors.Wrap(err, "creating directory %s", target)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return errors.Wrap(err, "creating parent directory")
			}

			//nolint:gosec // Mode is from trusted archive header
			outFile, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY, os.FileMode(header.Mode&0o777))
			if err != nil {
				return errors.Wrap(err, "creating file %s", target)
			}

			if _, err := io.Copy(outFile, io.LimitReader(tr, maxFileSize)); err != nil {
				outFile.Close()
				return errors.Wrap(err, "writing file %s", target)
			}
			outFile.Close()
		default:
			// Skip other entry types
		}
	}

	return nil
}

// extractZip extracts a .zip archive to the destination directory.
func extractZip(src string, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return errors.Wrap(err, "opening zip archive")
	}
	defer r.Close()

	for _, f := range r.File {
		// Reject symlinks
		if f.FileInfo().Mode()&os.ModeSymlink != 0 {
			return errors.New("archive contains a symlink: %s (rejected for security)", f.Name)
		}

		// Prevent path traversal
		if !isSafePath(dest, f.Name) {
			return errors.New("archive contains path traversal: %s", f.Name)
		}

		target := filepath.Join(dest, filepath.Clean(f.Name))

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return errors.Wrap(err, "creating directory %s", target)
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return errors.Wrap(err, "creating parent directory")
		}

		rc, err := f.Open()
		if err != nil {
			return errors.Wrap(err, "opening zip entry %s", f.Name)
		}

		// Strip setuid/setgid/sticky bits — keep only rwx permissions (matches tar extractor)
		outFile, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY, f.Mode()&os.ModePerm)
		if err != nil {
			rc.Close()
			return errors.Wrap(err, "creating file %s", target)
		}

		if _, err := io.Copy(outFile, io.LimitReader(rc, maxFileSize)); err != nil {
			outFile.Close()
			rc.Close()
			return errors.Wrap(err, "writing file %s", target)
		}

		outFile.Close()
		rc.Close()
	}

	return nil
}

// isSafePath checks that a file path from an archive stays within the destination directory.
func isSafePath(base, name string) bool {
	target := filepath.Join(base, filepath.Clean(name))
	return strings.HasPrefix(target, filepath.Clean(base)+string(os.PathSeparator))
}

// validateToolName rejects tool names that contain path separators or are relative path components.
func validateToolName(name string) error {
	if name == "" {
		return errors.New("tool name must not be empty")
	}
	if name == "." || name == ".." {
		return errors.New("invalid tool name: %q", name)
	}
	if filepath.Base(name) != name {
		return errors.New("invalid tool name: %q (must not contain path separators)", name)
	}
	return nil
}

// findBinary looks for an executable file matching the tool name in the given directory.
// It checks the root level and one level of nesting.
func findBinary(dir string, toolName string) (string, error) {
	// Check root level first
	rootPath := filepath.Join(dir, toolName)
	if info, err := os.Stat(rootPath); err == nil && !info.IsDir() {
		return rootPath, nil
	}

	// Walk to find it (one level deep max)
	var found string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if info.IsDir() {
			// Don't descend more than 2 levels deep
			rel, _ := filepath.Rel(dir, path)
			if strings.Count(rel, string(os.PathSeparator)) > 1 {
				return filepath.SkipDir
			}
			return nil
		}

		baseName := filepath.Base(path)
		// Match exact name or name without extension
		nameWithoutExt := strings.TrimSuffix(baseName, filepath.Ext(baseName))
		if baseName == toolName || nameWithoutExt == toolName {
			found = path
			return filepath.SkipAll
		}

		return nil
	})

	if err != nil {
		return "", errors.Wrap(err, "searching for binary")
	}

	if found == "" {
		return "", errors.New("binary %q not found in extracted archive", toolName)
	}

	return found, nil
}

// copyFile copies a file from src to dst.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return errors.Wrap(err, "opening source file")
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return errors.Wrap(err, "creating destination file")
	}

	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return errors.Wrap(err, "copying file")
	}

	if err := out.Sync(); err != nil {
		out.Close()
		return errors.Wrap(err, "syncing destination file")
	}

	if err := out.Close(); err != nil {
		return errors.Wrap(err, "closing destination file")
	}

	return nil
}
