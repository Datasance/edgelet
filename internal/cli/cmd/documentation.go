package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

func newDocumentationCommand(root *cobra.Command) *cobra.Command {
	cmd := &cobra.Command{
		Use:    "documentation",
		Short:  "Generate CLI reference documentation",
		Hidden: true,
	}

	generate := &cobra.Command{
		Use:   "generate",
		Short: "Generate documentation artifacts",
	}

	generate.AddCommand(
		newDocGenerateCommand(root, "md", generateMarkdown),
		newDocGenerateCommand(root, "man", generateManPages),
	)

	cmd.AddCommand(generate)
	return cmd
}

type docGenerator func(root *cobra.Command, outputDir string) error

func newDocGenerateCommand(root *cobra.Command, format string, gen docGenerator) *cobra.Command {
	var outputDir string
	c := &cobra.Command{
		Use:   format,
		Short: fmt.Sprintf("Generate %s documentation", format),
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if root == nil {
				return errors.New("root command is nil")
			}
			if outputDir == "" {
				return errors.New("--output is required")
			}
			return gen(root, outputDir)
		},
	}
	c.Flags().StringVar(&outputDir, "output", "", "Output directory")
	return c
}

func generateMarkdown(root *cobra.Command, outputDir string) error {
	if err := os.MkdirAll(outputDir, 0o755); err != nil { // #nosec G301 -- generated docs output must be world-readable
		return err
	}
	return doc.GenMarkdownTree(root, outputDir)
}

func generateManPages(root *cobra.Command, outputDir string) error {
	if err := os.MkdirAll(outputDir, 0o755); err != nil { // #nosec G301 -- generated docs output must be world-readable
		return err
	}
	header := &doc.GenManHeader{
		Title:   "EDGELET",
		Section: "1",
	}
	return doc.GenManTree(root, header, outputDir)
}
