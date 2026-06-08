package cmd

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
)

func completionCommandLong() string {
	return strings.TrimSpace(`Generate shell completion scripts for edgelet.

Install the script for your shell, then start a new session or source the file.

bash:
  edgelet completion bash | sudo tee /etc/bash_completion.d/edgelet

zsh:
  edgelet completion zsh > "${fpath[1]}/_edgelet"

fish:
  edgelet completion fish > ~/.config/fish/completions/edgelet.fish

Regenerate the packaged bash script with: make cli-completion`)
}

func completionCommandExamples() string {
	return strings.TrimSpace(`edgelet completion bash
  edgelet completion zsh
  edgelet completion fish`)
}

func newCompletionCommand(root *cobra.Command) *cobra.Command {
	cmd := &cobra.Command{
		Use:       "completion [bash|zsh|fish]",
		Short:     "Generate shell completion scripts",
		Long:      completionCommandLong(),
		Example:   completionCommandExamples(),
		ValidArgs: []string{"bash", "zsh", "fish"},
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
	var example string
	switch shell {
	case "bash":
		example = "edgelet completion bash | sudo tee /etc/bash_completion.d/edgelet"
	case "zsh":
		example = `edgelet completion zsh > "${fpath[1]}/_edgelet"`
	case "fish":
		example = "edgelet completion fish > ~/.config/fish/completions/edgelet.fish"
	}

	return &cobra.Command{
		Use:     shell,
		Short:   fmt.Sprintf("Generate %s completion script", shell),
		Example: example,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if root == nil {
				return errors.New("root command is nil")
			}
			return gen(cmd.OutOrStdout())
		},
	}
}
