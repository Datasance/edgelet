//go:build !linux

package cmd

import (
	"github.com/eclipse-iofog/edgelet/internal/cli/run"
	"github.com/spf13/cobra"
)

func newRuntimeCommand() *cobra.Command {
	return &cobra.Command{
		Use:    "runtime",
		Short:  "Embedded runtime data-plane operations (linux only)",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return run.NewCLIError(run.CodeInvalidArgument, "edgelet runtime is only supported on linux", nil)
		},
	}
}
