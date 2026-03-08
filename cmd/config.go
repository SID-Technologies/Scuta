package cmd

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/sid-technologies/scuta/lib/config"
	"github.com/sid-technologies/scuta/lib/errors"
	"github.com/sid-technologies/scuta/lib/output"
	"github.com/sid-technologies/scuta/lib/path"
	"github.com/sid-technologies/scuta/lib/suggest"

	"github.com/spf13/cobra"
)

func ConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage Scuta configuration",
		Long: `View and modify Scuta's configuration (~/.scuta/config.yaml).

Valid keys:
  update_interval   How often to check for updates (e.g. 24h, 12h)
  github_token      GitHub token for private repo access
  registry_url      Override the default remote registry URL`,
	}

	cmd.AddCommand(configListCmd())
	cmd.AddCommand(configGetCmd())
	cmd.AddCommand(configSetCmd())
	cmd.AddCommand(configResetCmd())

	return cmd
}

//nolint:gochecknoinits // Standard Cobra pattern
func init() {
	rootCmd.AddCommand(ConfigCmd())
}

func configListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "Show all configuration values",
		RunE:  runConfigList,
	}

	return cmd
}

func configGetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <key>",
		Short: "Get a configuration value",
		Args:  cobra.ExactArgs(1),
		RunE:  runConfigGet,
	}

	return cmd
}

func configSetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set a configuration value",
		Args:  cobra.ExactArgs(2),
		RunE:  runConfigSet,
	}

	return cmd
}

func configResetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reset <key>",
		Short: "Reset a configuration value to its default",
		Args:  cobra.ExactArgs(1),
		RunE:  runConfigReset,
	}

	return cmd
}

func runConfigList(_ *cobra.Command, _ []string) error {
	scutaDir, err := path.ScutaDir()
	if err != nil {
		return err
	}

	cfg, err := config.Load(scutaDir)
	if err != nil {
		return err
	}

	fields := cfg.FieldMap()
	keys := config.ValidKeys()
	sort.Strings(keys)

	if output.IsJSON() {
		type entry struct {
			Key          string `json:"key"`
			Value        string `json:"value"`
			DefaultValue string `json:"default"`
		}

		var entries []entry
		for _, k := range keys {
			entries = append(entries, entry{
				Key:          k,
				Value:        maskValue(k, fields[k]),
				DefaultValue: config.DefaultValue(k),
			})
		}

		output.JSON(entries)
		return nil
	}

	headers := []string{"KEY", "VALUE", "DEFAULT"}
	var rows []output.TableRow

	for _, k := range keys {
		rows = append(rows, output.TableRow{
			Columns: []string{k, maskValue(k, fields[k]), config.DefaultValue(k)},
		})
	}

	output.PrintTable(headers, rows)
	return nil
}

func runConfigGet(_ *cobra.Command, args []string) error {
	key := args[0]

	if !isValidKey(key) {
		return unknownKeyError(key)
	}

	scutaDir, err := path.ScutaDir()
	if err != nil {
		return err
	}

	cfg, err := config.Load(scutaDir)
	if err != nil {
		return err
	}

	value := cfg.FieldMap()[key]

	if output.IsJSON() {
		output.JSON(map[string]string{
			"key":   key,
			"value": value,
		})
		return nil
	}

	fmt.Println(value)
	return nil
}

func runConfigSet(_ *cobra.Command, args []string) error {
	key := args[0]
	value := args[1]

	if !isValidKey(key) {
		return unknownKeyError(key)
	}

	if err := validateValue(key, value); err != nil {
		return err
	}

	scutaDir, err := path.ScutaDir()
	if err != nil {
		return err
	}

	cfg, err := config.Load(scutaDir)
	if err != nil {
		return err
	}

	if err := cfg.SetField(key, value); err != nil {
		return errors.Wrap(err, "setting config value")
	}

	if err := config.Save(scutaDir, cfg); err != nil {
		return err
	}

	output.Success("Set %s = %s", key, value)
	return nil
}

func runConfigReset(_ *cobra.Command, args []string) error {
	key := args[0]

	if !isValidKey(key) {
		return unknownKeyError(key)
	}

	scutaDir, err := path.ScutaDir()
	if err != nil {
		return err
	}

	cfg, err := config.Load(scutaDir)
	if err != nil {
		return err
	}

	if err := cfg.ResetField(key); err != nil {
		return errors.Wrap(err, "resetting config value")
	}

	if err := config.Save(scutaDir, cfg); err != nil {
		return err
	}

	defaultVal := config.DefaultValue(key)
	if defaultVal == "" {
		output.Success("Reset %s (cleared)", key)
	} else {
		output.Success("Reset %s = %s", key, defaultVal)
	}

	return nil
}

func isValidKey(key string) bool {
	for _, k := range config.ValidKeys() {
		if k == key {
			return true
		}
	}
	return false
}

func unknownKeyError(key string) error {
	keys := config.ValidKeys()
	suggestion := suggest.FormatSuggestion(key, keys)
	if suggestion != "" {
		return errors.New("unknown config key %q — %s", key, suggestion)
	}
	return errors.New("unknown config key %q\nValid keys: %s", key, strings.Join(keys, ", "))
}

func validateValue(key, value string) error {
	switch key {
	case "update_interval":
		if _, err := time.ParseDuration(value); err != nil {
			return errors.New("invalid duration for update_interval: %q (examples: 12h, 30m, 24h)", value)
		}
	case "registry_url":
		if value == "local" {
			break
		}
		u, err := url.Parse(value)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return errors.New("invalid URL for registry_url: %q (must include scheme and host, or \"local\")", value)
		}
	default:
		// No validation for other keys (e.g. github_token)
	}
	return nil
}

func maskValue(key, value string) string {
	if key == "github_token" && value != "" {
		return "****"
	}
	return value
}
