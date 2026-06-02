//go:build !linux

package cmd

import (
	"github.com/datasance/edgelet/internal/cli/run"
	"github.com/spf13/cobra"
)

func newCgroupPreflightCommand() *cobra.Command {
	return &cobra.Command{
		Use:    "cgroup-preflight",
		Short:  "Cgroup delegation preflight (linux only)",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return run.NewCLIError(run.CodeInvalidArgument, "edgelet cgroup-preflight is only supported on linux", nil)
		},
	}
}
