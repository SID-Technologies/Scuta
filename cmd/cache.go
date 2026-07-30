package cmd

import (
	"fmt"

	"github.com/sid-technologies/scuta/lib/cache"
	"github.com/sid-technologies/scuta/lib/config"
	"github.com/sid-technologies/scuta/lib/output"
	"github.com/sid-technologies/scuta/lib/path"

	"github.com/spf13/cobra"
)

// CacheCmd groups download-cache maintenance commands.
func CacheCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cache",
		Short: "Inspect or clear the download cache",
		Long: `Manage the content-addressed download cache (~/.scuta/cache).

Only checksum-verified release assets are ever cached, keyed by their
SHA-256 digest, so a cache hit is as trustworthy as a fresh verified
download. Disable with: scuta config set disable_download_cache true`,
	}

	cmd.AddCommand(cacheInfoCmd())
	cmd.AddCommand(cacheClearCmd())

	return cmd
}

//nolint:gochecknoinits // Standard Cobra pattern
func init() {
	rootCmd.AddCommand(CacheCmd())
}

func cacheInfoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "info",
		Short: "Show download cache location, entry count, and size",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runCacheInfo()
		},
	}
}

func cacheClearCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "clear",
		Short: "Remove all cached downloads",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runCacheClear()
		},
	}
}

func runCacheInfo() error {
	scutaDir, err := path.ScutaDir()
	if err != nil {
		return err
	}

	c := cache.New(scutaDir)
	stats, err := c.Info()
	if err != nil {
		return err
	}

	enabled := true
	if cfg, cfgErr := config.LoadWithMerge(scutaDir); cfgErr == nil && cfg.DisableDownloadCache {
		enabled = false
	}

	output.Info("Location: %s", c.Dir())
	output.Info("Enabled:  %v", enabled)
	output.Info("Entries:  %d", stats.Entries)
	output.Info("Size:     %s", humanBytes(stats.TotalBytes))
	return nil
}

func runCacheClear() error {
	scutaDir, err := path.ScutaDir()
	if err != nil {
		return err
	}

	c := cache.New(scutaDir)
	stats, err := c.Info()
	if err != nil {
		return err
	}

	if err := c.Clear(); err != nil {
		return err
	}

	output.Success("Cleared %d cached download(s) (%s)", stats.Entries, humanBytes(stats.TotalBytes))
	return nil
}

// humanBytes formats a byte count using binary units.
func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
