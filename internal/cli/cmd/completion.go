package cmd

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
)

func completionCommandLong() string {
	return strings.TrimSpace(`Generate shell completion scripts for iofog-agent.

Install the script for your shell, then start a new session or source the file.

bash:
  iofog-agent completion bash | sudo tee /etc/bash_completion.d/iofog-agent

zsh:
  iofog-agent completion zsh > "${fpath[1]}/_iofog-agent"

fish:
  iofog-agent completion fish > ~/.config/fish/completions/iofog-agent.fish

Regenerate the packaged bash script with: make cli-completion`)
}

func completionCommandExamples() string {
	return strings.TrimSpace(`iofog-agent completion bash
  iofog-agent completion zsh
  iofog-agent completion fish`)
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
		example = "iofog-agent completion bash | sudo tee /etc/bash_completion.d/iofog-agent"
	case "zsh":
		example = `iofog-agent completion zsh > "${fpath[1]}/_iofog-agent"`
	case "fish":
		example = "iofog-agent completion fish > ~/.config/fish/completions/iofog-agent.fish"
	}

	return &cobra.Command{
		Use:     shell,
		Short:   fmt.Sprintf("Generate %s completion script", shell),
		Example: example,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if root == nil {
				return fmt.Errorf("root command is nil")
			}
			return gen(cmd.OutOrStdout())
		},
	}
}
