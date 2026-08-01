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

	"github.com/sid-technologies/scuta/lib/cache"
	"github.com/sid-technologies/scuta/lib/errors"
	"github.com/sid-technologies/scuta/lib/github"
	"github.com/sid-technologies/scuta/lib/output"
	"github.com/sid-technologies/scuta/lib/provenance"
)

// Installer manages downloading and installing tool binaries.
type Installer struct {
	github           *github.Client
	scutaDir         string
	binDir           string
	requireSignature bool
	signaturePubKey  []byte
	cache            *cache.Cache

	provenanceMode     provenance.Mode
	provenanceBackends []provenance.Backend
}

// InstallResult holds the outcome of an install operation.
type InstallResult struct {
	Version    string
	BinaryPath string
	// Sha256 is the SHA-256 of the installed binary, for recording in state.
	Sha256 string
	// Verified is true when the downloaded asset's checksum was verified
	// against the release's checksums file.
	Verified bool
	// Provenance lists the verification backends (e.g. "cosign", "slsa")
	// that positively verified the downloaded asset.
	Provenance []string
}

// InstallOpts provides extended options for tool installation.
// When set, these override the default GoReleaser conventions.
type InstallOpts struct {
	AssetTemplate string            // Template for asset name resolution
	BinName       string            // Binary name if different from tool name
	OSMap         map[string]string // OS name mappings (e.g., darwin -> Darwin)
	ArchMap       map[string]string // Arch name mappings (e.g., amd64 -> x86_64)
	VersionPrefix string            // Version prefix for tags (default "v", "none" for no prefix)
	BestEffort    bool              // If true, warn on missing checksums instead of failing
}

// New creates an Installer that installs to ~/.scuta/bin/.
func New(ghClient *github.Client, scutaDir string) *Installer {
	return &Installer{
		github:   ghClient,
		scutaDir: scutaDir,
		binDir:   filepath.Join(scutaDir, "bin"),
		cache:    cache.New(scutaDir),
	}
}

// NewWithBinDir creates an Installer that installs to a custom bin directory.
// Used for system-wide installs (e.g. /usr/local/bin).
func NewWithBinDir(ghClient *github.Client, scutaDir string, binDir string) *Installer {
	return &Installer{
		github:   ghClient,
		scutaDir: scutaDir,
		binDir:   binDir,
		cache:    cache.New(scutaDir),
	}
}

// SetSignatureVerification configures signature verification on the installer.
// When pubKey is non-empty, signatures will be checked after checksum verification.
func (inst *Installer) SetSignatureVerification(requireSig bool, pubKey []byte) {
	inst.requireSignature = requireSig
	inst.signaturePubKey = pubKey
}

// SetDownloadCache enables or disables the content-addressed download cache.
// It is enabled by default; disable via the disable_download_cache config key.
func (inst *Installer) SetDownloadCache(enabled bool) {
	if enabled {
		inst.cache = cache.New(inst.scutaDir)
		return
	}
	inst.cache = nil
}

// SetProvenanceVerification configures the optional post-download
// provenance backends (cosign keyless signatures, SLSA attestations).
// ModeOff (the default) disables them entirely.
func (inst *Installer) SetProvenanceVerification(mode provenance.Mode, identity provenance.CosignIdentity) {
	inst.provenanceMode = mode
	if mode == provenance.ModeOff {
		inst.provenanceBackends = nil
		return
	}
	inst.provenanceBackends = []provenance.Backend{
		provenance.NewCosign(identity),
		provenance.NewSLSA(),
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
		release, err = inst.github.GetReleaseTolerant(ctx, repo, applyVersionPrefix(targetVersion, ""))
	} else {
		release, err = inst.github.GetLatestRelease(ctx, repo)
	}
	if err != nil {
		return nil, errors.Wrap(err, "fetching release for %s", toolName)
	}

	version := github.NormalizeVersion(release.TagName)
	binaryPath := filepath.Join(inst.binDir, binaryName(toolName))

	// Check if already installed at this version
	if !force {
		if _, err := os.Stat(binaryPath); err == nil {
			output.Debugf("%s already exists at %s", toolName, binaryPath)
		}
	}

	// Find matching asset (heuristic matcher tries strict GoReleaser patterns
	// first, then falls back to looser OS/arch matching so tools using Rust-style
	// target triples like "aarch64-apple-darwin" resolve consistently on update).
	asset, err := github.FindAssetHeuristic(release.Assets, runtime.GOOS, runtime.GOARCH)
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

	// Fetch + verify (fail-closed: any checksum failure is an error unless --skip-verify)
	archivePath := filepath.Join(tmpDir, asset.Name)
	outcome, err := inst.fetchVerifiedAsset(ctx, release, repo, asset.Name, asset.BrowserDownloadURL, toolName, archivePath, skipVerify, false)
	if err != nil {
		return nil, err
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
	if err := os.MkdirAll(inst.binDir, 0o700); err != nil {
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

	return newInstallResult(version, binaryPath, outcome), nil
}

// InstallWithOpts downloads and installs a tool binary with extended options.
// This supports non-GoReleaser naming conventions via asset templates, custom binary names, etc.
func (inst *Installer) InstallWithOpts(ctx context.Context, toolName string, repo string, targetVersion string, force bool, skipVerify bool, opts InstallOpts) (*InstallResult, error) {
	if err := validateToolName(toolName); err != nil {
		return nil, err
	}

	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// Get the release (handle version prefix)
	var release *github.Release
	var err error

	if targetVersion != "" {
		tag := applyVersionPrefix(targetVersion, opts.VersionPrefix)
		release, err = inst.github.GetReleaseTolerant(ctx, repo, tag)
	} else {
		release, err = inst.github.GetLatestRelease(ctx, repo)
	}
	if err != nil {
		return nil, errors.Wrap(err, "fetching release for %s", toolName)
	}

	version := github.NormalizeVersion(release.TagName)

	// Determine the effective binary name
	effectiveBinName := toolName
	if opts.BinName != "" {
		effectiveBinName = opts.BinName
	}
	binaryPath := filepath.Join(inst.binDir, binaryName(effectiveBinName))

	// Check if already installed at this version
	if !force {
		if _, err := os.Stat(binaryPath); err == nil {
			output.Debugf("%s already exists at %s", toolName, binaryPath)
		}
	}

	// Find matching asset using template or heuristic
	assetOpts := github.AssetOptions{
		Template: opts.AssetTemplate,
		OSMap:    opts.OSMap,
		ArchMap:  opts.ArchMap,
		Version:  version,
		ToolName: toolName,
		BinName:  effectiveBinName,
	}

	asset, err := github.ResolveAsset(release.Assets, runtime.GOOS, runtime.GOARCH, assetOpts)
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

	// Fetch + verify (BestEffort softens missing checksums to warnings)
	archivePath := filepath.Join(tmpDir, asset.Name)
	outcome, err := inst.fetchVerifiedAsset(ctx, release, repo, asset.Name, asset.BrowserDownloadURL, toolName, archivePath, skipVerify, opts.BestEffort)
	if err != nil {
		return nil, err
	}

	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// Ensure bin directory exists
	if err := os.MkdirAll(inst.binDir, 0o700); err != nil {
		return nil, errors.Wrap(err, "creating bin directory")
	}

	// Handle raw binaries (no archive extraction needed)
	if github.IsRawBinary(asset.Name) {
		return inst.installRawBinary(archivePath, binaryPath, effectiveBinName, version, outcome)
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
	binarySrc, err := findBinaryWithName(extractDir, toolName, effectiveBinName)
	if err != nil {
		return nil, errors.Wrap(err, "finding binary in archive")
	}

	if ctx.Err() != nil {
		return nil, ctx.Err()
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

	output.Debugf("Installed %s %s to %s", effectiveBinName, version, binaryPath)

	return newInstallResult(version, binaryPath, outcome), nil
}

// fetchOutcome reports what verification actually happened for a fetched
// asset: whether the checksum matched the release checksums file, and which
// provenance backends (if any) verified it.
type fetchOutcome struct {
	checksumVerified bool
	provenance       []string
}

// newInstallResult builds an InstallResult, recording the SHA-256 of the
// installed binary so state can later detect out-of-band modification.
// Hashing failure is not fatal — the install itself already succeeded.
func newInstallResult(version, binaryPath string, outcome fetchOutcome) *InstallResult {
	sha, err := FileSHA256(binaryPath)
	if err != nil {
		output.Debugf("Could not hash installed binary %s: %v", binaryPath, err)
	}

	return &InstallResult{
		Version:    version,
		BinaryPath: binaryPath,
		Sha256:     sha,
		Verified:   outcome.checksumVerified,
		Provenance: outcome.provenance,
	}
}

// fetchVerifiedAsset obtains a release asset into destPath and applies
// checksum and signature verification. When a trusted checksum is known and
// the download cache is enabled, a verified cached copy is reused instead of
// re-downloading; fresh verified downloads are stored back into the cache.
//
// bestEffort softens missing-checksum conditions (no checksums file, no
// entry for this asset) to warnings. A checksum that is present but fails
// to match is always fatal, and unverified assets are never cached.
// It reports which verification actually happened via fetchOutcome.
func (inst *Installer) fetchVerifiedAsset(ctx context.Context, release *github.Release, repo, assetName, downloadURL, toolName, destPath string, skipVerify, bestEffort bool) (fetchOutcome, error) {
	expectedHash, err := inst.resolveExpectedHash(ctx, release, assetName, toolName, skipVerify, bestEffort)
	if err != nil {
		return fetchOutcome{}, err
	}

	fromCache := false
	if expectedHash != "" && inst.cache != nil {
		hit, cacheErr := inst.cache.Get(expectedHash, destPath)
		if cacheErr != nil {
			output.Debugf("Download cache lookup failed for %s: %v", assetName, cacheErr)
		} else if hit {
			output.Debugf("Using cached download for %s", assetName)
			fromCache = true
		}
	}

	if !fromCache {
		if err := inst.github.DownloadAsset(ctx, downloadURL, destPath); err != nil {
			return fetchOutcome{}, errors.Wrap(err, "downloading %s", assetName)
		}
	}

	if expectedHash != "" {
		if err := VerifyChecksum(destPath, expectedHash); err != nil {
			return fetchOutcome{}, errors.Wrap(err, "checksum verification failed for %s", toolName)
		}
		output.Debugf("Checksum verified for %s", assetName)

		// Cache writes are best-effort: a full disk must not fail the install.
		if !fromCache && inst.cache != nil {
			if err := inst.cache.Put(expectedHash, destPath); err != nil {
				output.Debugf("Could not cache download for %s: %v", assetName, err)
			}
		}
	}

	// Signature verification (when public key is configured)
	if len(inst.signaturePubKey) > 0 {
		if err := DownloadAndVerifySignature(ctx, inst.github, release, assetName, destPath, inst.signaturePubKey, inst.requireSignature); err != nil {
			return fetchOutcome{}, errors.Wrap(err, "signature verification failed for %s", toolName)
		}
	} else if inst.requireSignature {
		return fetchOutcome{}, errors.New("signature required but no public key configured (set signature_public_key in config)")
	}

	// Optional provenance backends (cosign, slsa) run last, over the same
	// bytes the checksum covered.
	verifiedBy, err := inst.VerifyProvenance(ctx, release, repo, assetName, destPath, skipVerify)
	if err != nil {
		return fetchOutcome{}, err
	}

	return fetchOutcome{checksumVerified: expectedHash != "", provenance: verifiedBy}, nil
}

// VerifyProvenance runs the configured provenance backends against a fetched
// asset and returns the names of backends that verified it. --skip-verify
// skips backends in auto mode but is rejected under require, mirroring how
// require_signature treats it. It is exported for direct (non-registry)
// installs, which manage their own download and checksum flow.
func (inst *Installer) VerifyProvenance(ctx context.Context, release *github.Release, repo, assetName, assetPath string, skipVerify bool) ([]string, error) {
	if inst.provenanceMode == provenance.ModeOff || inst.provenanceMode == "" {
		return nil, nil
	}

	if skipVerify {
		if inst.provenanceMode == provenance.ModeRequire {
			return nil, errors.New("--skip-verify cannot be used while provenance_verify is \"require\"")
		}
		output.Warning("Skipping provenance verification (--skip-verify)")
		return nil, nil
	}

	assets := make([]provenance.Asset, 0, len(release.Assets))
	for _, a := range release.Assets {
		assets = append(assets, provenance.Asset{Name: a.Name, URL: a.BrowserDownloadURL})
	}

	results, err := provenance.Run(ctx, inst.provenanceMode, inst.provenanceBackends, provenance.Request{
		Repo:      repo,
		Tag:       release.TagName,
		AssetName: assetName,
		AssetPath: assetPath,
		Assets:    assets,
		WorkDir:   filepath.Dir(assetPath),
		Download:  inst.github.DownloadAsset,
	})
	if err != nil {
		return nil, err
	}

	return provenance.VerifiedBackends(results), nil
}

// resolveExpectedHash fetches the release checksums file and returns the
// expected SHA-256 for assetName. It returns "" (verify nothing) when
// verification is skipped or when bestEffort downgrades a missing checksum
// to a warning; otherwise missing checksums fail closed.
func (inst *Installer) resolveExpectedHash(ctx context.Context, release *github.Release, assetName, toolName string, skipVerify, bestEffort bool) (string, error) {
	if skipVerify {
		output.Warning("Skipping checksum verification (--skip-verify)")
		return "", nil
	}

	checksums, csErr := inst.github.DownloadChecksums(ctx, release)
	switch {
	case csErr != nil:
		if !bestEffort {
			return "", errors.Wrap(csErr, "checksum verification failed for %s: could not download checksums", toolName)
		}
		output.Warning("Could not download checksums for %s: %v", toolName, csErr)
		return "", nil
	case checksums == nil:
		if !bestEffort {
			return "", errors.New("checksum verification failed for %s: no checksums file in release (use --skip-verify to override)", toolName)
		}
		output.Warning("No checksums file in release for %s — skipping verification", toolName)
		return "", nil
	}

	expectedHash, ok := checksums[assetName]
	if !ok {
		if !bestEffort {
			return "", errors.New("checksum verification failed for %s: no entry for %s in checksums file (use --skip-verify to override)", toolName, assetName)
		}
		output.Warning("No checksum entry for %s — skipping verification", assetName)
		return "", nil
	}

	return expectedHash, nil
}

// installRawBinary handles installing a raw binary (not an archive).
func (*Installer) installRawBinary(srcPath, binaryPath, binName, version string, outcome fetchOutcome) (*InstallResult, error) {
	tempPath := binaryPath + ".tmp"
	if err := copyFile(srcPath, tempPath); err != nil {
		os.Remove(tempPath)
		return nil, errors.Wrap(err, "installing raw binary")
	}

	if err := os.Chmod(tempPath, 0o755); err != nil {
		os.Remove(tempPath)
		return nil, errors.Wrap(err, "setting binary permissions")
	}

	if err := os.Rename(tempPath, binaryPath); err != nil {
		os.Remove(tempPath)
		return nil, errors.Wrap(err, "atomic rename of binary")
	}

	output.Debugf("Installed raw binary %s %s to %s", binName, version, binaryPath)

	return newInstallResult(version, binaryPath, outcome), nil
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
	if err := os.MkdirAll(inst.binDir, 0o700); err != nil {
		return nil, errors.Wrap(err, "creating bin directory")
	}

	// Atomic install: copy to temp, then rename
	binaryPath := filepath.Join(inst.binDir, binaryName(toolName))
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

	return newInstallResult(version, binaryPath, fetchOutcome{}), nil
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

	binaryPath := filepath.Join(inst.binDir, binaryName(toolName))
	return inst.removeManagedBinary(binaryPath, toolName)
}

// UninstallBinaryPath removes a previously installed binary by its recorded
// path. Use this when the installed binary name differs from the tool name
// (a custom "bin"), where deriving the path from the tool name would miss the
// real file. The path must live inside the managed bin directory.
func (inst *Installer) UninstallBinaryPath(binaryPath string) error {
	return inst.removeManagedBinary(binaryPath, filepath.Base(binaryPath))
}

// removeManagedBinary deletes binaryPath after confirming it is inside the
// installer's managed bin directory, guarding against path traversal or
// removing files outside Scuta's control. A missing file is not an error.
func (inst *Installer) removeManagedBinary(binaryPath, label string) error {
	cleaned := filepath.Clean(binaryPath)
	if parent := filepath.Dir(cleaned); parent != filepath.Clean(inst.binDir) {
		return errors.New("refusing to remove %q outside managed bin dir %q", cleaned, inst.binDir)
	}

	if err := os.Remove(cleaned); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return errors.Wrap(err, "removing binary %s", label)
	}

	output.Debugf("Removed %s from %s", label, cleaned)
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

// BinaryName returns the platform-appropriate binary name.
// On Windows, it appends ".exe" if not already present.
func BinaryName(toolName string) string {
	return binaryName(toolName)
}

// binaryName returns the platform-appropriate binary name.
// On Windows, it appends ".exe" if not already present.
func binaryName(toolName string) string {
	if runtime.GOOS == "windows" && !strings.HasSuffix(strings.ToLower(toolName), ".exe") {
		return toolName + ".exe"
	}
	return toolName
}

// findBinary looks for an executable file matching the tool name in the given directory.
// It checks the root level and one level of nesting.
// On Windows, it also checks for the tool name with an .exe extension.
// As a fallback, it matches files prefixed with the tool name (e.g., "pilum_v1.0.0_darwin_arm64").
func findBinary(dir string, toolName string) (string, error) {
	// Build candidate names: exact name, and on Windows also with .exe
	candidates := []string{toolName}
	if runtime.GOOS == "windows" {
		candidates = append(candidates, toolName+".exe")
	}

	// Check root level first (exact match)
	for _, name := range candidates {
		rootPath := filepath.Join(dir, name)
		if info, err := os.Stat(rootPath); err == nil && !info.IsDir() {
			return rootPath, nil
		}
	}

	// Walk to find it (one level deep max)
	var found string
	var prefixMatch string   // fallback: file starting with toolName_
	var executables []string // fallback: any executable file (e.g. repo "ripgrep" ships binary "rg")
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
		nameWithoutExt := strings.TrimSuffix(baseName, filepath.Ext(baseName))

		// An exact filename match is the strongest signal (a bare "rg").
		if baseName == toolName {
			found = path
			return filepath.SkipAll
		}

		// A match only after stripping an extension is trusted only when the
		// file still looks executable. This prevents shell-completion or doc
		// files like "rg.bash"/"rg.1" from being mistaken for the "rg" binary.
		if nameWithoutExt == toolName && looksLikeBinary(baseName, info.Mode()) {
			found = path
			return filepath.SkipAll
		}

		// On Windows, also match the ".exe" form explicitly.
		if runtime.GOOS == "windows" && strings.EqualFold(baseName, toolName+".exe") {
			found = path
			return filepath.SkipAll
		}

		// Fallback: match files prefixed with toolName_ (e.g., "pilum_v1.0.0_darwin_arm64")
		if prefixMatch == "" && strings.HasPrefix(baseName, toolName+"_") && !info.IsDir() {
			prefixMatch = path
		}

		// Fallback: track executable files whose name isn't obviously a script/doc.
		// Handles tools where the binary name differs from the repo name.
		if looksLikeBinary(baseName, info.Mode()) {
			executables = append(executables, path)
		}

		return nil
	})

	if err != nil {
		return "", errors.Wrap(err, "searching for binary")
	}

	if found != "" {
		return found, nil
	}

	// Use prefix match as fallback
	if prefixMatch != "" {
		return prefixMatch, nil
	}

	// Last resort: if the archive contains exactly one executable, use it.
	// Ambiguous (multiple executables) stays an error to avoid installing the wrong file.
	if len(executables) == 1 {
		return executables[0], nil
	}

	return "", errors.New("binary %q not found in extracted archive", toolName)
}

// looksLikeBinary reports whether a file is plausibly the tool's executable:
// it must have an execute bit set and must not have an extension associated with
// scripts, docs, or completions (which are sometimes shipped executable).
func looksLikeBinary(baseName string, mode os.FileMode) bool {
	if runtime.GOOS != "windows" && mode&0o111 == 0 {
		return false
	}

	ext := strings.ToLower(filepath.Ext(baseName))
	switch ext {
	case ".sh", ".bash", ".zsh", ".fish", ".ps1", ".bat", ".cmd",
		".txt", ".md", ".1", ".json", ".yaml", ".yml", ".toml", ".cfg",
		".so", ".dylib", ".dll":
		return false
	}

	if runtime.GOOS == "windows" {
		return ext == ".exe"
	}

	return true
}

// applyVersionPrefix adds the appropriate prefix to a version string for tag lookup.
// If prefix is "none", no prefix is added. If prefix is empty, "v" is used (default).
// Otherwise, the specified prefix is used.
func applyVersionPrefix(version, prefix string) string {
	// Strip existing "v" prefix if present
	version = strings.TrimPrefix(version, "v")

	if prefix == "none" {
		return version
	}

	if prefix == "" {
		return "v" + version
	}

	return prefix + version
}

// findBinaryWithName looks for a binary matching either the tool name or an alternate bin name.
// It checks the bin name first (if different from tool name), then falls back to the tool name.
func findBinaryWithName(dir, toolName, binName string) (string, error) {
	// If bin name differs from tool name, try the bin name first
	if binName != "" && binName != toolName {
		if path, err := findBinary(dir, binName); err == nil {
			return path, nil
		}
	}

	// Fall back to tool name
	return findBinary(dir, toolName)
}

// CopyFile copies a file from src to dst.
func CopyFile(src, dst string) error {
	return copyFile(src, dst)
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
