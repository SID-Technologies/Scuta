package installer

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"

	"github.com/sid-technologies/scuta/lib/errors"
	"github.com/sid-technologies/scuta/lib/github"
	"github.com/sid-technologies/scuta/lib/output"
	"github.com/sid-technologies/scuta/lib/sigverify"
)

// BundleManifest describes the contents of a bundle archive.
//
// Version 1 bundles are single-platform: OS/Arch at the top level and one
// Asset+Checksum per tool. Version 2 bundles may span multiple platforms:
// Platforms lists "<os>_<arch>" keys and each tool carries per-platform
// assets (stored in the archive under "<platform>/<asset>"). Version 2
// checksums are always present (computed at creation time).
type BundleManifest struct {
	Version int                       `json:"version"`
	Tools   map[string]BundleToolInfo `json:"tools"`

	// Version 1 fields (single-platform bundles).
	OS   string `json:"os,omitempty"`
	Arch string `json:"arch,omitempty"`

	// Version 2 fields.
	Platforms []string `json:"platforms,omitempty"`
}

// BundleToolInfo describes a single tool in the bundle.
type BundleToolInfo struct {
	Version string `json:"version"`
	Repo    string `json:"repo,omitempty"`

	// Version 1 fields (single-platform bundles).
	Asset    string `json:"asset,omitempty"`
	Checksum string `json:"checksum,omitempty"`

	// Version 2: platform key ("<os>_<arch>") -> asset.
	Assets map[string]BundleAsset `json:"assets,omitempty"`
}

// BundleAsset is one platform's archive for a tool in a version 2 bundle.
type BundleAsset struct {
	Asset    string `json:"asset"`
	Checksum string `json:"checksum"`
}

// BundleSpec selects what to bundle for one tool.
type BundleSpec struct {
	Repo    string
	Version string // empty = latest
}

// BundlePlatform is a target OS/arch pair for a bundle.
type BundlePlatform struct {
	OS   string
	Arch string
}

// Key returns the manifest/platform-directory key, e.g. "linux_amd64".
func (p BundlePlatform) Key() string {
	return p.OS + "_" + p.Arch
}

// HostPlatform returns the platform of the running binary.
func HostPlatform() BundlePlatform {
	return BundlePlatform{OS: runtime.GOOS, Arch: runtime.GOARCH}
}

// CreateBundleOpts configures CreateBundle.
type CreateBundleOpts struct {
	// Tools maps tool name to what to bundle. Required.
	Tools map[string]BundleSpec
	// Platforms to include. Empty = host platform only.
	Platforms []BundlePlatform
	// SigningKeyPEM, when non-empty, signs manifest.json and embeds the
	// detached signature as manifest.json.sig inside the bundle.
	SigningKeyPEM []byte
}

const (
	bundleManifestFile  = "manifest.json"
	bundleSignatureFile = "manifest.json.sig"
	bundleVersion       = 2
)

// CreateBundle downloads the requested tools for every target platform and
// packages them into a single tar.gz bundle with a version 2 manifest.
//
// Every asset's SHA-256 is computed locally and recorded in the manifest.
// When the upstream release publishes a checksums file, the download is
// additionally verified against it (mismatch is fatal); when it does not,
// the locally computed checksum still pins the bundle contents so
// verification at install time catches any post-creation tampering.
func CreateBundle(
	ctx context.Context,
	ghClient *github.Client,
	opts CreateBundleOpts,
	outputPath string,
) (*BundleManifest, error) {
	if len(opts.Tools) == 0 {
		return nil, errors.New("no tools to bundle")
	}

	platforms := opts.Platforms
	if len(platforms) == 0 {
		platforms = []BundlePlatform{HostPlatform()}
	}

	tmpDir, err := os.MkdirTemp("", "scuta-bundle-*")
	if err != nil {
		return nil, errors.Wrap(err, "creating temp directory")
	}
	defer os.RemoveAll(tmpDir)

	manifest := &BundleManifest{
		Version: bundleVersion,
		Tools:   make(map[string]BundleToolInfo),
	}
	for _, p := range platforms {
		manifest.Platforms = append(manifest.Platforms, p.Key())
	}
	sort.Strings(manifest.Platforms)

	// Deterministic tool order for readable output.
	names := make([]string, 0, len(opts.Tools))
	for name := range opts.Tools {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		spec := opts.Tools[name]

		info, err := bundleTool(ctx, ghClient, name, spec, platforms, tmpDir)
		if err != nil {
			return nil, err
		}
		manifest.Tools[name] = *info
	}

	// Write manifest
	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, errors.Wrap(err, "marshaling manifest")
	}
	manifestPath := filepath.Join(tmpDir, bundleManifestFile)
	if err := os.WriteFile(manifestPath, manifestData, 0o644); err != nil { //nolint:gosec // manifest is not secret
		return nil, errors.Wrap(err, "writing manifest")
	}

	// Sign the manifest (optional). The manifest pins every asset by
	// SHA-256, so a valid manifest signature transitively covers all
	// bundle contents.
	if len(opts.SigningKeyPEM) > 0 {
		sig, err := sigverify.Sign(manifestData, opts.SigningKeyPEM)
		if err != nil {
			return nil, errors.Wrap(err, "signing bundle manifest")
		}
		sigPath := filepath.Join(tmpDir, bundleSignatureFile)
		if err := os.WriteFile(sigPath, sig, 0o644); err != nil { //nolint:gosec // signature is public
			return nil, errors.Wrap(err, "writing manifest signature")
		}
	}

	// Create the bundle tar.gz
	if err := createBundleTarGz(tmpDir, outputPath, manifest, len(opts.SigningKeyPEM) > 0); err != nil {
		return nil, errors.Wrap(err, "creating bundle archive")
	}

	return manifest, nil
}

// bundleTool downloads one tool for every target platform into
// tmpDir/<platform>/ and returns its manifest entry.
func bundleTool(
	ctx context.Context,
	ghClient *github.Client,
	name string,
	spec BundleSpec,
	platforms []BundlePlatform,
	tmpDir string,
) (*BundleToolInfo, error) {
	output.Info("Fetching %s...", name)

	var release *github.Release
	var err error
	if spec.Version != "" {
		release, err = ghClient.GetReleaseTolerant(ctx, spec.Repo, applyVersionPrefix(spec.Version, ""))
	} else {
		release, err = ghClient.GetLatestRelease(ctx, spec.Repo)
	}
	if err != nil {
		return nil, errors.Wrap(err, "fetching release for %s", name)
	}

	version := github.NormalizeVersion(release.TagName)

	// Upstream checksums are best-effort: when present, downloads are
	// verified against them; either way we record our own SHA-256.
	checksums, _ := ghClient.DownloadChecksums(ctx, release)

	info := &BundleToolInfo{
		Version: version,
		Repo:    spec.Repo,
		Assets:  make(map[string]BundleAsset, len(platforms)),
	}

	for _, p := range platforms {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		asset, err := github.FindAssetHeuristic(release.Assets, p.OS, p.Arch)
		if err != nil {
			return nil, errors.Wrap(err, "finding %s asset for %s", p.Key(), name)
		}

		platformDir := filepath.Join(tmpDir, p.Key())
		if err := os.MkdirAll(platformDir, 0o755); err != nil {
			return nil, errors.Wrap(err, "creating platform directory")
		}

		assetPath := filepath.Join(platformDir, asset.Name)
		if err := ghClient.DownloadAsset(ctx, asset.BrowserDownloadURL, assetPath); err != nil {
			return nil, errors.Wrap(err, "downloading %s for %s", asset.Name, p.Key())
		}

		if upstream, ok := checksums[asset.Name]; ok {
			if err := VerifyChecksum(assetPath, upstream); err != nil {
				return nil, errors.Wrap(err, "upstream checksum verification failed for %s", name)
			}
		}

		sum, err := fileSHA256(assetPath)
		if err != nil {
			return nil, errors.Wrap(err, "hashing %s", asset.Name)
		}

		info.Assets[p.Key()] = BundleAsset{Asset: asset.Name, Checksum: sum}
		output.Success("Bundled %s %s (%s)", name, version, p.Key())
	}

	return info, nil
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
	manifestData, err := os.ReadFile(manifestPath) //nolint:gosec // path built from our temp dir
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

// BundleAssetForHost resolves the archive path (relative to the extracted
// bundle directory) and expected checksum of a tool for the running
// platform. It handles both manifest versions:
//   - v1: single-platform bundle; the caller gets the flat asset path. A
//     mismatched bundle platform is an error.
//   - v2: the "<os>_<arch>" entry is selected; missing platform is an error.
func BundleAssetForHost(manifest *BundleManifest, toolName string) (relPath string, checksum string, err error) {
	info, ok := manifest.Tools[toolName]
	if !ok {
		return "", "", errors.New("tool %q not in bundle manifest", toolName)
	}

	if manifest.Version <= 1 {
		if manifest.OS != "" && (manifest.OS != runtime.GOOS || manifest.Arch != runtime.GOARCH) {
			return "", "", errors.New(
				"bundle was created for %s/%s but this machine is %s/%s",
				manifest.OS, manifest.Arch, runtime.GOOS, runtime.GOARCH,
			)
		}
		return info.Asset, info.Checksum, nil
	}

	key := HostPlatform().Key()
	asset, ok := info.Assets[key]
	if !ok {
		return "", "", errors.New(
			"bundle has no %s build of %s (available: %s)",
			key, toolName, fmt.Sprintf("%v", manifest.Platforms),
		)
	}
	return filepath.Join(key, asset.Asset), asset.Checksum, nil
}

// BundleVerifyResult reports one verified item in a bundle.
type BundleVerifyResult struct {
	Tool     string
	Platform string
	Asset    string
	Err      error
}

// VerifyBundleSignature checks the embedded manifest signature of an
// extracted bundle against a PEM public key.
//
// Returns (signed=false, nil) when the bundle carries no signature and
// requireSignature is false. A present-but-invalid signature is always an
// error, regardless of requireSignature.
func VerifyBundleSignature(bundleDir string, publicKeyPEM []byte, requireSignature bool) (signed bool, err error) {
	sigPath := filepath.Join(bundleDir, bundleSignatureFile)
	sig, err := os.ReadFile(sigPath) //nolint:gosec // path built from our temp dir
	if err != nil {
		if os.IsNotExist(err) {
			if requireSignature {
				return false, errors.New("bundle is not signed (require_signature is enabled)")
			}
			return false, nil
		}
		return false, errors.Wrap(err, "reading bundle signature")
	}

	if len(publicKeyPEM) == 0 {
		return true, errors.New("bundle is signed but no public key is configured (set signature_public_key or pass --key)")
	}

	manifestData, err := os.ReadFile(filepath.Join(bundleDir, bundleManifestFile)) //nolint:gosec // path built from our temp dir
	if err != nil {
		return true, errors.Wrap(err, "reading bundle manifest")
	}

	if err := sigverify.Verify(manifestData, sig, publicKeyPEM); err != nil {
		return true, errors.Wrap(err, "bundle signature verification failed")
	}
	return true, nil
}

// VerifyBundleChecksums verifies every asset in an extracted bundle against
// the manifest checksums. It returns one result per asset, including
// failures (missing files, checksum mismatches, or absent checksums).
func VerifyBundleChecksums(bundleDir string, manifest *BundleManifest) []BundleVerifyResult {
	var results []BundleVerifyResult

	names := make([]string, 0, len(manifest.Tools))
	for name := range manifest.Tools {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		info := manifest.Tools[name]

		if manifest.Version <= 1 {
			res := BundleVerifyResult{Tool: name, Platform: manifest.OS + "_" + manifest.Arch, Asset: info.Asset}
			res.Err = verifyBundleAsset(bundleDir, info.Asset, info.Checksum)
			results = append(results, res)
			continue
		}

		platforms := make([]string, 0, len(info.Assets))
		for key := range info.Assets {
			platforms = append(platforms, key)
		}
		sort.Strings(platforms)

		for _, key := range platforms {
			asset := info.Assets[key]
			res := BundleVerifyResult{Tool: name, Platform: key, Asset: asset.Asset}
			res.Err = verifyBundleAsset(bundleDir, filepath.Join(key, asset.Asset), asset.Checksum)
			results = append(results, res)
		}
	}

	return results
}

// verifyBundleAsset checks one asset file against its recorded checksum.
func verifyBundleAsset(bundleDir, relPath, checksum string) error {
	assetPath := filepath.Join(bundleDir, relPath)
	if _, err := os.Stat(assetPath); err != nil {
		return errors.New("asset missing from bundle: %s", relPath)
	}
	if checksum == "" {
		return errors.New("no checksum recorded for %s", relPath)
	}
	return VerifyChecksum(assetPath, checksum)
}

// fileSHA256 returns the lowercase hex SHA-256 of a file.
func fileSHA256(path string) (string, error) {
	f, err := os.Open(path) //nolint:gosec // path built from our temp dir
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

// createBundleTarGz creates a tar.gz containing the manifest, optional
// signature, and all tool assets (under their platform directories for
// version 2 bundles).
func createBundleTarGz(sourceDir string, outputPath string, manifest *BundleManifest, signed bool) error {
	f, err := os.Create(outputPath) //nolint:gosec // user-supplied output path by design
	if err != nil {
		return errors.Wrap(err, "creating output file")
	}
	defer f.Close()

	gw := gzip.NewWriter(f)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	// Add manifest (and its signature, when signed)
	if err := addFileToTar(tw, filepath.Join(sourceDir, bundleManifestFile), bundleManifestFile); err != nil {
		return errors.Wrap(err, "adding manifest to bundle")
	}
	if signed {
		if err := addFileToTar(tw, filepath.Join(sourceDir, bundleSignatureFile), bundleSignatureFile); err != nil {
			return errors.Wrap(err, "adding manifest signature to bundle")
		}
	}

	// Add each tool's assets
	for _, info := range manifest.Tools {
		if manifest.Version <= 1 {
			if err := addFileToTar(tw, filepath.Join(sourceDir, info.Asset), info.Asset); err != nil {
				return errors.Wrap(err, "adding %s to bundle", info.Asset)
			}
			continue
		}
		for key, asset := range info.Assets {
			rel := filepath.Join(key, asset.Asset)
			if err := addFileToTar(tw, filepath.Join(sourceDir, rel), key+"/"+asset.Asset); err != nil {
				return errors.Wrap(err, "adding %s to bundle", rel)
			}
		}
	}

	return nil
}

// addFileToTar adds a single file to a tar writer.
func addFileToTar(tw *tar.Writer, filePath string, nameInTar string) error {
	f, err := os.Open(filePath) //nolint:gosec // path built from our temp dir
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
