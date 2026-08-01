package provenance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/sid-technologies/scuta/lib/errors"
)

// defaultOIDCIssuer is the OIDC issuer for GitHub-Actions keyless signing,
// which is what GoReleaser's cosign integration produces.
const defaultOIDCIssuer = "https://token.actions.githubusercontent.com"

// CosignIdentity pins the expected certificate identity for keyless
// verification. Zero values derive sane defaults from the release repo:
// issuer = GitHub Actions, identity = any workflow in that repository.
type CosignIdentity struct {
	// IdentityRegexp overrides --certificate-identity-regexp.
	IdentityRegexp string
	// OIDCIssuer overrides --certificate-oidc-issuer.
	OIDCIssuer string
}

// Cosign verifies release assets against cosign keyless signature material
// published alongside them: sigstore bundles, detached .sig + certificate
// pairs, or a signed checksums file that vouches for the asset.
type Cosign struct {
	identity CosignIdentity

	// lookPath and run are injectable for tests.
	lookPath func(file string) (string, error)
	run      func(ctx context.Context, name string, args ...string) ([]byte, error)
}

// NewCosign returns a cosign backend using the cosign CLI from PATH.
func NewCosign(identity CosignIdentity) *Cosign {
	return &Cosign{
		identity: identity,
		lookPath: exec.LookPath,
		run:      runCommand,
	}
}

// Name implements Backend.
func (*Cosign) Name() string { return "cosign" }

// cosignMaterial describes the signature material found for one asset.
type cosignMaterial struct {
	kind string // "bundle", "signature", "checksums-bundle", "checksums-signature"
	// blob is the release asset whose bytes the signature covers: either
	// the asset itself or a checksums file that lists it.
	blob   Asset
	bundle *Asset
	sig    *Asset
	cert   *Asset
}

// Verify implements Backend.
func (c *Cosign) Verify(ctx context.Context, req Request) (Result, error) {
	skip := func(reason string) Result {
		return Result{Backend: c.Name(), Skipped: true, Reason: reason}
	}

	if _, err := c.lookPath("cosign"); err != nil {
		return skip("cosign not found in PATH"), nil
	}

	material := findCosignMaterial(req)
	if material == nil {
		return skip("no cosign signature material in release"), nil
	}

	identityRegexp, issuer, err := c.resolveIdentity(req.Repo)
	if err != nil {
		return skip(err.Error()), nil
	}

	blobPath, args, err := c.buildVerifyArgs(ctx, req, material, identityRegexp, issuer)
	if err != nil {
		return Result{}, err
	}

	out, err := c.run(ctx, "cosign", args...)
	if err != nil {
		return Result{}, errors.New("cosign rejected %s: %v: %s", material.blob.Name, err, summarizeOutput(out))
	}

	// When the signed blob is a checksums file rather than the asset
	// itself, the signature only helps if that file vouches for the exact
	// bytes we downloaded.
	if material.blob.Name != req.AssetName {
		if err := checksumsVouchForAsset(blobPath, req.AssetName, req.AssetPath); err != nil {
			return Result{}, err
		}
	}

	return Result{
		Backend:  c.Name(),
		Verified: true,
		Detail:   material.kind + " " + materialName(material),
	}, nil
}

// resolveIdentity returns the certificate identity constraints, deriving
// defaults from the release repository when not overridden.
func (c *Cosign) resolveIdentity(repo string) (identityRegexp string, issuer string, err error) {
	identityRegexp = c.identity.IdentityRegexp
	if identityRegexp == "" {
		if repo == "" {
			return "", "", errors.New("cannot derive signer identity: no source repository (set cosign_identity_regexp)")
		}
		identityRegexp = "^https://github\\.com/" + regexp.QuoteMeta(repo) + "/"
	}

	issuer = c.identity.OIDCIssuer
	if issuer == "" {
		issuer = defaultOIDCIssuer
	}

	return identityRegexp, issuer, nil
}

// buildVerifyArgs downloads the companion material into the work dir and
// returns the local blob path plus the cosign verify-blob argument list.
func (*Cosign) buildVerifyArgs(ctx context.Context, req Request, material *cosignMaterial, identityRegexp, issuer string) (string, []string, error) {
	blobPath := req.AssetPath
	if material.blob.Name != req.AssetName {
		var err error
		blobPath, err = downloadCompanion(ctx, req, material.blob)
		if err != nil {
			return "", nil, err
		}
	}

	args := []string{"verify-blob"}

	switch {
	case material.bundle != nil:
		bundlePath, err := downloadCompanion(ctx, req, *material.bundle)
		if err != nil {
			return "", nil, err
		}
		args = append(args, "--bundle", bundlePath)
		if strings.HasSuffix(strings.ToLower(material.bundle.Name), ".sigstore.json") {
			args = append(args, "--new-bundle-format")
		}
	default:
		sigPath, err := downloadCompanion(ctx, req, *material.sig)
		if err != nil {
			return "", nil, err
		}
		certPath, err := downloadCompanion(ctx, req, *material.cert)
		if err != nil {
			return "", nil, err
		}
		args = append(args, "--signature", sigPath, "--certificate", certPath)
	}

	args = append(args,
		"--certificate-identity-regexp", identityRegexp,
		"--certificate-oidc-issuer", issuer,
		blobPath,
	)

	return blobPath, args, nil
}

// findCosignMaterial locates signature material for the asset, preferring
// per-asset material over a signed checksums file.
func findCosignMaterial(req Request) *cosignMaterial {
	if m := materialForBlob(req.Assets, req.AssetName, ""); m != nil {
		return m
	}

	for _, name := range []string{"checksums.txt", "SHA256SUMS", "SHA256SUMS.txt"} {
		blob := findAsset(req.Assets, name)
		if blob == nil {
			continue
		}
		if m := materialForBlob(req.Assets, blob.Name, "checksums-"); m != nil {
			m.blob = *blob
			return m
		}
	}

	return nil
}

// materialForBlob finds a sigstore bundle or a .sig + certificate pair for
// the named blob. kindPrefix distinguishes checksums-based material.
func materialForBlob(assets []Asset, blobName, kindPrefix string) *cosignMaterial {
	blob := findAsset(assets, blobName)
	if blob == nil {
		return nil
	}

	for _, suffix := range []string{".sigstore.json", ".bundle", ".sigstore"} {
		if bundle := findAsset(assets, blobName+suffix); bundle != nil {
			return &cosignMaterial{kind: kindPrefix + "bundle", blob: *blob, bundle: bundle}
		}
	}

	sig := findAsset(assets, blobName+".sig")
	if sig == nil {
		return nil
	}
	for _, suffix := range []string{".pem", ".cert", ".crt"} {
		if cert := findAsset(assets, blobName+suffix); cert != nil {
			return &cosignMaterial{kind: kindPrefix + "signature", blob: *blob, sig: sig, cert: cert}
		}
	}

	// A bare .sig without a certificate is a key-based detached signature;
	// that path is handled by the signature_public_key flow, not cosign.
	return nil
}

// materialName names the primary evidence file for the success detail.
func materialName(m *cosignMaterial) string {
	if m.bundle != nil {
		return m.bundle.Name
	}
	return m.sig.Name
}

// downloadCompanion fetches a companion asset into the work directory.
func downloadCompanion(ctx context.Context, req Request, a Asset) (string, error) {
	dest := filepath.Join(req.WorkDir, filepath.Base(a.Name))
	if err := req.Download(ctx, a.URL, dest); err != nil {
		return "", errors.Wrap(err, "downloading %s", a.Name)
	}
	return dest, nil
}

// checksumsVouchForAsset confirms the verified checksums file lists the
// asset with the hash of the bytes actually downloaded.
func checksumsVouchForAsset(checksumsPath, assetName, assetPath string) error {
	data, err := os.ReadFile(checksumsPath) //nolint:gosec // path is inside the scuta-owned work dir
	if err != nil {
		return errors.Wrap(err, "reading verified checksums file")
	}

	expected := ""
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		if strings.TrimLeft(fields[1], "*") == assetName {
			expected = strings.ToLower(fields[0])
			break
		}
	}
	if expected == "" {
		return errors.New("signed checksums file has no entry for %s", assetName)
	}

	f, err := os.Open(assetPath) //nolint:gosec // path is inside the scuta-owned work dir
	if err != nil {
		return errors.Wrap(err, "opening asset for hash comparison")
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return errors.Wrap(err, "hashing asset")
	}
	actual := hex.EncodeToString(h.Sum(nil))

	if !strings.EqualFold(actual, expected) {
		return errors.New("signed checksums file does not vouch for %s: expected %s, got %s", assetName, expected, actual)
	}

	return nil
}
