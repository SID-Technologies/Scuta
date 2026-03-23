package installer

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"

	"github.com/sid-technologies/scuta/lib/errors"
	"github.com/sid-technologies/scuta/lib/github"
	"github.com/sid-technologies/scuta/lib/output"
)

// BundleManifest describes the contents of a bundle archive.
type BundleManifest struct {
	Version int                       `json:"version"`
	Tools   map[string]BundleToolInfo `json:"tools"`
	OS      string                    `json:"os"`
	Arch    string                    `json:"arch"`
}

// BundleToolInfo describes a single tool in the bundle.
type BundleToolInfo struct {
	Version  string `json:"version"`
	Asset    string `json:"asset"`
	Checksum string `json:"checksum,omitempty"`
}

const bundleManifestFile = "manifest.json"

// CreateBundle downloads all specified tools and packages them into a single tar.gz bundle.
func CreateBundle(
	ctx context.Context,
	ghClient *github.Client,
	tools map[string]string, // name -> repo
	outputPath string,
) (*BundleManifest, error) {
	tmpDir, err := os.MkdirTemp("", "scuta-bundle-*")
	if err != nil {
		return nil, errors.Wrap(err, "creating temp directory")
	}
	defer os.RemoveAll(tmpDir)

	manifest := &BundleManifest{
		Version: 1,
		Tools:   make(map[string]BundleToolInfo),
		OS:      runtime.GOOS,
		Arch:    runtime.GOARCH,
	}

	for name, repo := range tools {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		output.Info("Fetching %s...", name)

		// Get latest release
		release, err := ghClient.GetLatestRelease(ctx, repo)
		if err != nil {
			return nil, errors.Wrap(err, "fetching release for %s", name)
		}

		version := github.NormalizeVersion(release.TagName)

		// Find matching asset
		asset, err := github.FindAssetAuto(release.Assets)
		if err != nil {
			return nil, errors.Wrap(err, "finding asset for %s", name)
		}

		// Download asset
		assetPath := filepath.Join(tmpDir, asset.Name)
		if err := ghClient.DownloadAsset(ctx, asset.BrowserDownloadURL, assetPath); err != nil {
			return nil, errors.Wrap(err, "downloading %s", name)
		}

		// Get checksum if available
		checksums, _ := ghClient.DownloadChecksums(ctx, release)
		checksum := ""
		if checksums != nil {
			checksum = checksums[asset.Name]
		}

		manifest.Tools[name] = BundleToolInfo{
			Version:  version,
			Asset:    asset.Name,
			Checksum: checksum,
		}

		output.Success("Bundled %s %s", name, version)
	}

	// Write manifest
	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, errors.Wrap(err, "marshaling manifest")
	}
	manifestPath := filepath.Join(tmpDir, bundleManifestFile)
	if err := os.WriteFile(manifestPath, manifestData, 0o644); err != nil {
		return nil, errors.Wrap(err, "writing manifest")
	}

	// Create the bundle tar.gz
	if err := createBundleTarGz(tmpDir, outputPath, manifest); err != nil {
		return nil, errors.Wrap(err, "creating bundle archive")
	}

	return manifest, nil
}

// ExtractBundle extracts a bundle tar.gz and returns the manifest + temp directory.
// The caller is responsible for cleaning up the temp directory.
func ExtractBundle(bundlePath string) (*BundleManifest, string, error) {
	tmpDir, err := os.MkdirTemp("", "scuta-bundle-extract-*")
	if err != nil {
		return nil, "", errors.Wrap(err, "creating temp directory")
	}

	if err := extractTarGz(bundlePath, tmpDir); err != nil {
		os.RemoveAll(tmpDir)
		return nil, "", errors.Wrap(err, "extracting bundle")
	}

	// Read manifest
	manifestPath := filepath.Join(tmpDir, bundleManifestFile)
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		os.RemoveAll(tmpDir)
		return nil, "", errors.Wrap(err, "reading bundle manifest")
	}

	var manifest BundleManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		os.RemoveAll(tmpDir)
		return nil, "", errors.Wrap(err, "parsing bundle manifest")
	}

	return &manifest, tmpDir, nil
}

// createBundleTarGz creates a tar.gz containing the manifest and all tool assets.
func createBundleTarGz(sourceDir string, outputPath string, manifest *BundleManifest) error {
	f, err := os.Create(outputPath)
	if err != nil {
		return errors.Wrap(err, "creating output file")
	}
	defer f.Close()

	gw := gzip.NewWriter(f)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	// Add manifest
	if err := addFileToTar(tw, filepath.Join(sourceDir, bundleManifestFile), bundleManifestFile); err != nil {
		return errors.Wrap(err, "adding manifest to bundle")
	}

	// Add each tool's asset
	for _, info := range manifest.Tools {
		if err := addFileToTar(tw, filepath.Join(sourceDir, info.Asset), info.Asset); err != nil {
			return errors.Wrap(err, "adding %s to bundle", info.Asset)
		}
	}

	return nil
}

// addFileToTar adds a single file to a tar writer.
func addFileToTar(tw *tar.Writer, filePath string, nameInTar string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return errors.Wrap(err, "opening file")
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return errors.Wrap(err, "stat file")
	}

	header, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return errors.Wrap(err, "creating tar header")
	}
	header.Name = nameInTar

	if err := tw.WriteHeader(header); err != nil {
		return errors.Wrap(err, "writing tar header")
	}

	if _, err := io.Copy(tw, f); err != nil {
		return errors.Wrap(err, "writing file to tar")
	}

	return nil
}
