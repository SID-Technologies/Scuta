package cmd

import (
	"github.com/sid-technologies/scuta/lib/errors"
	"github.com/sid-technologies/scuta/lib/output"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

func docsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "docs",
		Short:  "Generate documentation",
		Hidden: true,
	}

	cmd.AddCommand(docsManCmd())
	cmd.AddCommand(docsMarkdownCmd())

	return cmd
}

func docsManCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "man",
		Short: "Generate man pages",
		RunE:  runDocsMan,
	}

	cmd.Flags().StringP("output-dir", "o", "./man", "Output directory for man pages")

	return cmd
}

func docsMarkdownCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "markdown",
		Short: "Generate markdown documentation",
		RunE:  runDocsMarkdown,
	}

	cmd.Flags().StringP("output-dir", "o", "./docs/cli", "Output directory for markdown docs")

	return cmd
}

//nolint:gochecknoinits // Standard Cobra pattern
func init() {
	rootCmd.AddCommand(docsCmd())
}

func runDocsMan(cmd *cobra.Command, _ []string) error {
	outputDir, _ := cmd.Flags().GetString("output-dir")

	header := &doc.GenManHeader{
		Title:   "SCUTA",
		Section: "1",
		Source:  "Scuta " + version,
		Manual:  "Scuta Manual",
	}

	if err := doc.GenManTree(rootCmd, header, outputDir); err != nil {
		return errors.Wrap(err, "generating man pages")
	}

	output.Success("Man pages generated in %s", outputDir)
	return nil
}

func runDocsMarkdown(cmd *cobra.Command, _ []string) error {
	outputDir, _ := cmd.Flags().GetString("output-dir")

	if err := doc.GenMarkdownTree(rootCmd, outputDir); err != nil {
		return errors.Wrap(err, "generating markdown docs")
	}

	output.Success("Markdown docs generated in %s", outputDir)
	return nil
}
