package cmd

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

func newCompletionCommand(root *cobra.Command) *cobra.Command {
	cmd := &cobra.Command{
		Use:    "completion",
		Short:  "Generate shell completion scripts",
		Hidden: true,
	}

	cmd.AddCommand(
		newShellCompletionCommand(root, "bash", func(w io.Writer) error {
			return root.GenBashCompletion(w)
		}),
		newShellCompletionCommand(root, "zsh", func(w io.Writer) error {
			return root.GenZshCompletion(w)
		}),
		newShellCompletionCommand(root, "fish", func(w io.Writer) error {
			return root.GenFishCompletion(w, true)
		}),
	)

	return cmd
}

func newShellCompletionCommand(root *cobra.Command, shell string, gen func(w io.Writer) error) *cobra.Command {
	return &cobra.Command{
		Use:   shell,
		Short: fmt.Sprintf("Generate %s completion script", shell),
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if root == nil {
				return fmt.Errorf("root command is nil")
			}
			return gen(cmd.OutOrStdout())
		},
	}
}
