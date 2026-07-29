package cmd

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/sid-technologies/scuta/lib/config"
	"github.com/sid-technologies/scuta/lib/errors"
	"github.com/sid-technologies/scuta/lib/output"
	"github.com/sid-technologies/scuta/lib/path"
	"github.com/sid-technologies/scuta/lib/sigverify"

	"github.com/spf13/cobra"
)

// AdminCmd groups registry-operator tooling: signing key generation and
// detached signing/verification of metadata files (registry, policy,
// remote config).
func AdminCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "admin",
		Short: "Registry operator tools (keygen, sign, verify)",
		Long: `Tools for registry operators to sign metadata files.

Clients verify signed metadata when signature_public_key is set; with
require_signed_metadata enabled they refuse unsigned metadata entirely.

Typical flow:

  scuta admin keygen --out scuta-signing
  scuta admin sign registry.yaml --key scuta-signing.key
  # publish registry.yaml and registry.yaml.sig side by side`,
	}

	cmd.AddCommand(adminKeygenCmd())
	cmd.AddCommand(adminSignCmd())
	cmd.AddCommand(adminVerifyCmd())

	return cmd
}

//nolint:gochecknoinits // Standard Cobra pattern
func init() {
	rootCmd.AddCommand(AdminCmd())
}

func adminKeygenCmd() *cobra.Command {
	var outPrefix string

	cmd := &cobra.Command{
		Use:   "keygen",
		Short: "Generate an Ed25519 signing key pair",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runAdminKeygen(outPrefix)
		},
	}

	cmd.Flags().StringVar(&outPrefix, "out", "scuta-signing", "Output path prefix (writes <prefix>.key and <prefix>.pub)")

	return cmd
}

func runAdminKeygen(outPrefix string) error {
	keyPath := outPrefix + ".key"
	pubPath := outPrefix + ".pub"

	for _, p := range []string{keyPath, pubPath} {
		if _, err := os.Stat(p); err == nil {
			return errors.New("refusing to overwrite existing file %s", p)
		}
	}

	pub, priv, err := sigverify.GenerateEd25519Keys()
	if err != nil {
		return errors.Wrap(err, "generating key pair")
	}

	if err := os.WriteFile(keyPath, priv, 0o600); err != nil {
		return errors.Wrap(err, "writing private key")
	}
	if err := os.WriteFile(pubPath, pub, 0o644); err != nil { //nolint:gosec // public key is public
		return errors.Wrap(err, "writing public key")
	}

	output.Success("Wrote %s (private — keep offline) and %s", keyPath, pubPath)
	output.Info("Clients trust it via: scuta config set signature_public_key \"$(cat %s)\"", pubPath)

	return nil
}

func adminSignCmd() *cobra.Command {
	var keyPath string
	var outPath string

	cmd := &cobra.Command{
		Use:   "sign <file>",
		Short: "Create a detached signature for a file",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return runAdminSign(args[0], keyPath, outPath)
		},
	}

	cmd.Flags().StringVar(&keyPath, "key", "", "Path to the private key (required)")
	cmd.Flags().StringVar(&outPath, "output", "", "Signature output path (default <file>.sig)")
	_ = cmd.MarkFlagRequired("key")

	return cmd
}

func runAdminSign(filePath, keyPath, outPath string) error {
	data, err := os.ReadFile(filePath) //nolint:gosec // user-supplied path by design
	if err != nil {
		return errors.Wrap(err, "reading file to sign")
	}

	keyPEM, err := os.ReadFile(keyPath) //nolint:gosec // user-supplied path by design
	if err != nil {
		return errors.Wrap(err, "reading private key")
	}

	sig, err := sigverify.Sign(data, keyPEM)
	if err != nil {
		return errors.Wrap(err, "signing %s", filePath)
	}

	if outPath == "" {
		outPath = filePath + ".sig"
	}
	if err := os.WriteFile(outPath, sig, 0o644); err != nil { //nolint:gosec // signature is public
		return errors.Wrap(err, "writing signature")
	}

	output.Success("Signed %s → %s", filePath, outPath)
	output.Info("Publish %s next to %s", filepath.Base(outPath), filepath.Base(filePath))

	return nil
}

func adminVerifyCmd() *cobra.Command {
	var sigPath string
	var pubkeyPath string

	cmd := &cobra.Command{
		Use:   "verify <file>",
		Short: "Verify a file against its detached signature",
		Long: `Verify a file against its detached signature.

The public key is read from --pubkey if given, otherwise from the
signature_public_key config value (local + system config).`,
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return runAdminVerify(args[0], sigPath, pubkeyPath)
		},
	}

	cmd.Flags().StringVar(&sigPath, "sig", "", "Signature path (default <file>.sig)")
	cmd.Flags().StringVar(&pubkeyPath, "pubkey", "", "Public key path (default: signature_public_key from config)")

	return cmd
}

func runAdminVerify(filePath, sigPath, pubkeyPath string) error {
	data, err := os.ReadFile(filePath) //nolint:gosec // user-supplied path by design
	if err != nil {
		return errors.Wrap(err, "reading file to verify")
	}

	if sigPath == "" {
		sigPath = filePath + ".sig"
	}
	sig, err := os.ReadFile(sigPath) //nolint:gosec // user-supplied path by design
	if err != nil {
		return errors.Wrap(err, "reading signature %s", sigPath)
	}

	pubPEM, err := resolveVerifyKey(pubkeyPath)
	if err != nil {
		return err
	}

	if err := sigverify.Verify(data, sig, pubPEM); err != nil {
		return errors.Wrap(err, "verifying %s", filePath)
	}

	output.Success("Signature valid: %s (sig: %s)", filePath, sigPath)

	return nil
}

// resolveVerifyKey loads the public key from an explicit path, or falls back
// to the signature_public_key configured locally or system-wide.
func resolveVerifyKey(pubkeyPath string) ([]byte, error) {
	if pubkeyPath != "" {
		pubPEM, err := os.ReadFile(pubkeyPath) //nolint:gosec // user-supplied path by design
		if err != nil {
			return nil, errors.Wrap(err, "reading public key %s", pubkeyPath)
		}
		return pubPEM, nil
	}

	scutaDir, err := path.ScutaDir()
	if err != nil {
		return nil, errors.Wrap(err, "resolving scuta dir")
	}

	trust := config.LoadTrusted(scutaDir)
	if strings.TrimSpace(trust.SignaturePublicKey) == "" {
		return nil, errors.New("no public key: pass --pubkey or set signature_public_key in config")
	}

	return []byte(trust.SignaturePublicKey), nil
}
