//go:build !linux

package cmd

import (
	"github.com/datasance/edgelet/internal/cli/run"
	"github.com/spf13/cobra"
)

func newShutdownCommand() *cobra.Command {
	return &cobra.Command{
		Use:    "shutdown",
		Short:  "Control-plane stop for init systems (linux only)",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return run.NewCLIError(run.CodeInvalidArgument, "edgelet shutdown is only supported on linux", nil)
		},
	}
}
