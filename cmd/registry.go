package cmd

import (
	"fmt"
	"sort"
	"strings"

	"github.com/sid-technologies/scuta/lib/exitcodes"
	"github.com/sid-technologies/scuta/lib/output"
	"github.com/sid-technologies/scuta/lib/path"
	"github.com/sid-technologies/scuta/lib/registry"

	"github.com/spf13/cobra"
)

func RegistryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "registry",
		Short: "Manage the local tool registry",
		Long: `Add, remove, and list tools in your local registry (~/.scuta/local.yaml).

Local tools are merged with the main registry and take precedence on name
conflicts, letting you override remote tools or add entirely new ones.`,
	}

	cmd.AddCommand(registryAddCmd())
	cmd.AddCommand(registryRemoveCmd())
	cmd.AddCommand(registryListCmd())

	return cmd
}

//nolint:gochecknoinits // Standard Cobra pattern
func init() {
	rootCmd.AddCommand(RegistryCmd())
}

func registryAddCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add <name>",
		Short: "Add a tool to the local registry",
		Long: `Adds a tool entry to ~/.scuta/local.yaml. Use --force to overwrite
an existing local entry.`,
		Args: cobra.ExactArgs(1),
		RunE: runRegistryAdd,
	}

	cmd.Flags().String("repo", "", "GitHub repository (owner/repo)")
	cmd.Flags().String("description", "", "Tool description")
	cmd.Flags().String("depends-on", "", "Comma-separated list of dependencies")
	cmd.Flags().Bool("force", false, "Overwrite existing local entry")
	_ = cmd.MarkFlagRequired("repo")

	return cmd
}

func registryRemoveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove a tool from the local registry",
		Args:  cobra.ExactArgs(1),
		RunE:  runRegistryRemove,
	}

	return cmd
}

func registryListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List local registry entries",
		Long: `Shows tools defined in your local registry (~/.scuta/local.yaml).
Use --all to show the full merged view with source information.`,
		RunE: runRegistryList,
	}

	cmd.Flags().Bool("all", false, "Show full merged registry with source column")

	return cmd
}

func runRegistryAdd(cmd *cobra.Command, args []string) error {
	name := args[0]
	repo, _ := cmd.Flags().GetString("repo")
	description, _ := cmd.Flags().GetString("description")
	dependsOn, _ := cmd.Flags().GetString("depends-on")
	force, _ := cmd.Flags().GetBool("force")

	scutaDir, err := path.ScutaDir()
	if err != nil {
		return err
	}

	local, err := registry.LoadLocal(scutaDir)
	if err != nil {
		return err
	}

	// Check if tool already exists in local registry
	if _, exists := local.Get(name); exists && !force {
		return exitcodes.NewError(exitcodes.Config, fmt.Sprintf("%q already exists in local registry (use --force to overwrite)", name))
	}

	tool := registry.Tool{
		Repo:        repo,
		Description: description,
	}

	if dependsOn != "" {
		tool.DependsOn = strings.Split(dependsOn, ",")
		for i := range tool.DependsOn {
			tool.DependsOn[i] = strings.TrimSpace(tool.DependsOn[i])
		}
	}

	local.Tools[name] = tool

	if err := registry.SaveLocal(scutaDir, local); err != nil {
		return err
	}

	output.Success("Added %q to local registry", name)
	return nil
}

func runRegistryRemove(_ *cobra.Command, args []string) error {
	name := args[0]

	scutaDir, err := path.ScutaDir()
	if err != nil {
		return err
	}

	local, err := registry.LoadLocal(scutaDir)
	if err != nil {
		return err
	}

	if _, exists := local.Get(name); !exists {
		output.Warning("%q is not in the local registry", name)
		return nil
	}

	delete(local.Tools, name)

	if err := registry.SaveLocal(scutaDir, local); err != nil {
		return err
	}

	output.Success("Removed %q from local registry", name)
	return nil
}

func runRegistryList(cmd *cobra.Command, _ []string) error {
	allFlag, _ := cmd.Flags().GetBool("all")

	if allFlag {
		return listMergedRegistry()
	}

	return listLocalRegistry()
}

func listLocalRegistry() error {
	scutaDir, err := path.ScutaDir()
	if err != nil {
		return err
	}

	local, err := registry.LoadLocal(scutaDir)
	if err != nil {
		return err
	}

	names := local.Names()
	sort.Strings(names)

	if len(names) == 0 {
		output.Info("No local tools defined. Use 'scuta registry add' to add one.")
		return nil
	}

	if output.IsJSON() {
		var tools []output.ToolInfo
		for _, name := range names {
			tool, _ := local.Get(name)
			tools = append(tools, output.ToolInfo{
				Name:        name,
				Description: tool.Description,
				Repo:        tool.Repo,
				DependsOn:   tool.DependsOn,
			})
		}

		output.JSON(tools)
		return nil
	}

	headers := []string{"TOOL", "REPO", "DESCRIPTION"}
	var rows []output.TableRow

	for _, name := range names {
		tool, _ := local.Get(name)
		rows = append(rows, output.TableRow{
			Columns: []string{name, tool.Repo, tool.Description},
		})
	}

	output.PrintTable(headers, rows)
	return nil
}

func listMergedRegistry() error {
	reg, err := registry.Load()
	if err != nil {
		return err
	}

	names := reg.Names()
	sort.Strings(names)

	if output.IsJSON() {
		var tools []output.ToolInfo
		for _, name := range names {
			tool, _ := reg.Get(name)
			tools = append(tools, output.ToolInfo{
				Name:        name,
				Description: tool.Description,
				Repo:        tool.Repo,
				Source:      reg.Source(name),
				DependsOn:   tool.DependsOn,
			})
		}

		output.JSON(tools)
		return nil
	}

	headers := []string{"TOOL", "REPO", "SOURCE", "DESCRIPTION"}
	var rows []output.TableRow

	for _, name := range names {
		tool, _ := reg.Get(name)
		rows = append(rows, output.TableRow{
			Columns: []string{name, tool.Repo, reg.Source(name), tool.Description},
		})
	}

	output.PrintTable(headers, rows)
	return nil
}
