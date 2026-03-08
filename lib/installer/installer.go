// Package installer handles downloading, verifying, and installing tool binaries.
package installer

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
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
func (inst *Installer) Install(toolName string, repo string, targetVersion string, force bool) (*InstallResult, error) {
	// Get the release
	var release *github.Release
	var err error

	if targetVersion != "" {
		release, err = inst.github.GetRelease(repo, targetVersion)
	} else {
		release, err = inst.github.GetLatestRelease(repo)
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
	if err := inst.github.DownloadAsset(asset.BrowserDownloadURL, archivePath); err != nil {
		return nil, errors.Wrap(err, "downloading %s", asset.Name)
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

	// Ensure bin directory exists
	if err := os.MkdirAll(inst.binDir, 0o755); err != nil {
		return nil, errors.Wrap(err, "creating bin directory")
	}

	// Copy binary to bin directory
	if err := copyFile(binarySrc, binaryPath); err != nil {
		return nil, errors.Wrap(err, "installing binary")
	}

	// Make executable
	if err := os.Chmod(binaryPath, 0o755); err != nil {
		return nil, errors.Wrap(err, "setting binary permissions")
	}

	output.Debugf("Installed %s %s to %s", toolName, version, binaryPath)

	return &InstallResult{
		Version:    version,
		BinaryPath: binaryPath,
	}, nil
}

// Uninstall removes a tool binary from the bin directory.
func (inst *Installer) Uninstall(toolName string) error {
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

		// Prevent path traversal
		target := filepath.Join(dest, filepath.Clean(header.Name))
		if !strings.HasPrefix(target, filepath.Clean(dest)+string(os.PathSeparator)) {
			continue
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return errors.Wrap(err, "creating directory %s", target)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return errors.Wrap(err, "creating parent directory")
			}

			outFile, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY, os.FileMode(header.Mode))
			if err != nil {
				return errors.Wrap(err, "creating file %s", target)
			}

			//nolint:gosec // Size is bounded by GitHub's asset limits
			if _, err := io.Copy(outFile, tr); err != nil {
				outFile.Close()
				return errors.Wrap(err, "writing file %s", target)
			}
			outFile.Close()
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
		// Prevent path traversal
		target := filepath.Join(dest, filepath.Clean(f.Name))
		if !strings.HasPrefix(target, filepath.Clean(dest)+string(os.PathSeparator)) {
			continue
		}

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

		outFile, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY, f.Mode())
		if err != nil {
			rc.Close()
			return errors.Wrap(err, "creating file %s", target)
		}

		//nolint:gosec // Size is bounded by GitHub's asset limits
		if _, err := io.Copy(outFile, rc); err != nil {
			outFile.Close()
			rc.Close()
			return errors.Wrap(err, "writing file %s", target)
		}

		outFile.Close()
		rc.Close()
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
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return errors.Wrap(err, "copying file")
	}

	return nil
}
