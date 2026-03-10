package cmd

import (
	"fmt"
	"sort"
	"strings"

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
		var entries []output.ConfigEntry
		for _, k := range keys {
			entries = append(entries, output.ConfigEntry{
				Key:          k,
				Value:        config.MaskValue(k, fields[k]),
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
			Columns: []string{k, config.MaskValue(k, fields[k]), config.DefaultValue(k)},
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
	masked := config.MaskValue(key, value)

	if output.IsJSON() {
		output.JSON(map[string]string{
			"key":   key,
			"value": masked,
		})
		return nil
	}

	fmt.Println(masked)
	return nil
}

func runConfigSet(_ *cobra.Command, args []string) error {
	key := args[0]
	value := args[1]

	if !isValidKey(key) {
		return unknownKeyError(key)
	}

	if err := config.ValidateValue(key, value); err != nil {
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

	output.Success("Set %s = %s", key, config.MaskValue(key, value))
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
