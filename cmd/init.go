package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sid-technologies/scuta/lib/auth"
	"github.com/sid-technologies/scuta/lib/config"
	"github.com/sid-technologies/scuta/lib/exitcodes"
	"github.com/sid-technologies/scuta/lib/output"
	"github.com/sid-technologies/scuta/lib/path"
	"github.com/sid-technologies/scuta/lib/prompt"
	"github.com/sid-technologies/scuta/lib/shellutil"

	"github.com/spf13/cobra"
)

// InitCmd returns the init command.
func InitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Setup Scuta on a new machine",
		Long: `Creates ~/.scuta/ directory structure, detects GitHub authentication
(gh CLI or token), adds ~/.scuta/bin/ to PATH, and prints next steps.

With --from, the machine is bootstrapped non-interactively from an org
config URL: the URL is saved as config_url and the org config (registry,
policy, security settings) is fetched and applied on every run. Pass
--key with the org's public key to verify the config's signature; the
key is installed as the local trust root BEFORE anything is fetched, and
require_signed_metadata is enabled so all remote metadata fails closed
from then on.

Idempotent — safe to run multiple times.`,
		RunE: runInit,
	}

	cmd.Flags().String("from", "", "Org config URL to bootstrap from (non-interactive)")
	cmd.Flags().String("key", "", "Path to the org's public key (PEM) used to verify the org config")

	return cmd
}

//nolint:gochecknoinits // Standard Cobra pattern
func init() {
	rootCmd.AddCommand(InitCmd())
}

func runInit(cmd *cobra.Command, _ []string) error {
	fromURL, _ := cmd.Flags().GetString("from")
	keyPath, _ := cmd.Flags().GetString("key")
	if keyPath != "" && fromURL == "" {
		return exitcodes.NewError(exitcodes.InvalidArgs, "--key requires --from")
	}

	output.Header("Scuta Setup")

	// 1. Create directory structure
	scutaDir, err := path.EnsureDir()
	if err != nil {
		return err
	}
	output.Success("Created %s", scutaDir)

	// 2. Configure: org bootstrap (--from, non-interactive) or first-run prompts.
	if fromURL != "" {
		if err := bootstrapOrgConfig(scutaDir, fromURL, keyPath); err != nil {
			return err
		}
	} else if err := ensureLocalConfig(scutaDir); err != nil {
		return err
	}

	// 3. Detect GitHub auth
	token := auth.ResolveTokenWithConfig(scutaDir)
	if token != "" {
		output.Success("GitHub authentication detected")
	} else {
		output.Warning("No GitHub token found — set SCUTA_GITHUB_TOKEN or install gh CLI")
	}

	// 4. Check if bin dir is in PATH
	binDir, err := path.BinDir()
	if err != nil {
		return err
	}

	shell := shellutil.DetectShell()

	if shellutil.IsInPath(binDir) {
		output.Success("%s is in PATH", binDir)
	} else {
		output.Warning("%s is not in PATH", binDir)
		shellutil.PrintPathInstructions(binDir, shell)
	}

	// 5. Install shell completions. Prompt only in interactive mode; with
	// --from the whole run must work unattended (CI, fleet provisioning).
	if completionsInstalled(shell) {
		output.Success("Shell completions already installed")
	} else if fromURL != "" {
		output.Info("Install shell completions later with: scuta completion %s", shell)
	} else if shell != "sh" {
		reader := prompt.NewReader(bufio.NewReader(os.Stdin))
		answer, err := reader.Ask("Install shell completions? (Y/n)", "Y")
		if err == nil && (answer == "Y" || answer == "y" || answer == "yes" || answer == "") {
			if err := installCompletions(shell); err != nil {
				output.Warning("Failed to install completions: %v", err)
			}
		}
	}

	// 6. Print next steps
	output.Header("Next Steps")
	fmt.Println("  scuta install --all    Install all available tools")
	fmt.Println("  scuta list             See available tools")
	fmt.Println("  scuta doctor           Verify everything is working")
	fmt.Println()

	return nil
}

// ensureLocalConfig creates the initial config interactively on first run.
func ensureLocalConfig(scutaDir string) error {
	configPath := scutaDir + "/config.yaml"
	_, statErr := os.Stat(configPath)

	if statErr == nil {
		output.Success("Config already exists")
		return nil
	}

	if os.IsNotExist(statErr) {
		cfg, cfgErr := promptInitialConfig()
		if cfgErr != nil {
			return cfgErr
		}
		if cfgErr = config.Save(scutaDir, cfg); cfgErr != nil {
			return cfgErr
		}
		output.Success("Created config")
	}

	return nil
}

// bootstrapOrgConfig points this machine at an org config URL and fetches it
// once to validate. Ordering is security-critical: the trust root from --key
// is applied locally BEFORE the first fetch, so even the bootstrap download
// is verified — there is no unverified first hop unless the operator chose
// to bootstrap without a key.
func bootstrapOrgConfig(scutaDir, configURL, keyPath string) error {
	// Start from the file's actual contents, not Load()'s defaults: baking
	// defaults (e.g. update_interval 24h) into the local config would shadow
	// the org's values forever, since local overrides remote on merge.
	cfg := config.Config{Version: config.CurrentConfigVersion}
	if _, statErr := os.Stat(filepath.Join(scutaDir, "config.yaml")); statErr == nil {
		loaded, err := config.Load(scutaDir)
		if err != nil {
			return err
		}
		cfg = loaded
	}

	if keyPath != "" {
		pem, err := os.ReadFile(keyPath) //nolint:gosec // user-supplied path by design
		if err != nil {
			return exitcodes.NewError(exitcodes.IO, fmt.Sprintf("reading public key: %v", err))
		}
		if !strings.Contains(string(pem), "PUBLIC KEY") {
			return exitcodes.NewError(exitcodes.InvalidArgs, fmt.Sprintf("%s does not look like a PEM public key", keyPath))
		}
		cfg.SignaturePublicKey = strings.TrimSpace(string(pem))
		// A distributed key implies signed metadata: fail closed from now on.
		cfg.RequireSignedMetadata = true
	}

	// Resolve the trust root the same way the rest of Scuta does (defaults +
	// system + local), then overlay the key we were just handed.
	trust := config.LoadTrusted(scutaDir)
	if cfg.SignaturePublicKey != "" {
		trust.SignaturePublicKey = cfg.SignaturePublicKey
	}
	if cfg.RequireSignedMetadata {
		trust.RequireSignedMetadata = true
	}

	remote, err := config.FetchRemote(scutaDir, configURL, trust)
	if err != nil {
		return exitcodes.NewError(exitcodes.Network, fmt.Sprintf("fetching org config: %v", err))
	}
	if remote.SignaturePublicKey != "" {
		output.Warning("Org config contains signature_public_key — ignored (the trust root must be set locally)")
	}

	cfg.ConfigURL = configURL
	if err := config.Save(scutaDir, cfg); err != nil {
		return err
	}

	if trust.SignaturePublicKey != "" && trust.RequireSignedMetadata {
		output.Success("Org config verified and applied from %s", configURL)
	} else {
		output.Success("Org config applied from %s", configURL)
		output.Warning("Unsigned bootstrap — pass --key <pub.pem> so the org config is verified")
	}
	printOrgSettings(remote)

	return nil
}

// printOrgSettings summarizes what the fetched org config controls so the
// operator can see exactly what the URL now manages.
func printOrgSettings(remote config.Config) {
	rows := [][2]string{
		{"registry_url", remote.RegistryURL},
		{"policy_url", remote.PolicyURL},
		{"github_base_url", remote.GithubBaseURL},
		{"update_interval", remote.UpdateInterval},
		{"audit_log_destination", remote.AuditLogDestination},
	}
	if remote.RequireSignature {
		rows = append(rows, [2]string{"require_signature", "true"})
	}
	if remote.RequireSignedMetadata {
		rows = append(rows, [2]string{"require_signed_metadata", "true"})
	}
	if remote.DisableDownloadCache {
		rows = append(rows, [2]string{"disable_download_cache", "true"})
	}

	printed := false
	for _, row := range rows {
		if row[1] == "" {
			continue
		}
		if !printed {
			output.Info("Org config manages:")
			printed = true
		}
		fmt.Printf("  %-24s %s\n", row[0], row[1])
	}
	if !printed {
		output.Info("Org config sets no overrides yet — it will apply as soon as the org publishes settings")
	}
}

// promptInitialConfig runs an interactive setup to build the initial config.
func promptInitialConfig() (config.Config, error) {
	cfg := config.DefaultConfig()
	reader := prompt.NewReader(bufio.NewReader(os.Stdin))

	mode, err := reader.Select("Registry mode", []prompt.Option{
		{
			Key:         "public",
			Label:       "Public (default)",
			Description: "Use the official SID registry — no auth needed",
		},
		{
			Key:         "private",
			Label:       "Private",
			Description: "Use a private GitHub-hosted registry (requires token)",
		},
		{
			Key:         "local",
			Label:       "Local only",
			Description: "No remote registry — manage tools manually via 'scuta registry add'",
		},
	}, "public")
	if err != nil {
		return cfg, err
	}

	switch mode {
	case "private":
		url, err := reader.Ask("Registry URL", "")
		if err != nil {
			return cfg, err
		}
		if url != "" {
			cfg.RegistryURL = url
		}

		token, err := reader.Ask("GitHub token (or set SCUTA_GITHUB_TOKEN later)", "")
		if err != nil {
			return cfg, err
		}
		if token != "" {
			cfg.GithubToken = token
		}
	case "local":
		// Set a sentinel so the remote fetch is skipped
		cfg.RegistryURL = "local"
	default:
		// "public" uses defaults — no config changes needed
	}

	interval, err := reader.Ask("Update check interval", cfg.UpdateInterval)
	if err != nil {
		return cfg, err
	}
	cfg.UpdateInterval = interval

	return cfg, nil
}
