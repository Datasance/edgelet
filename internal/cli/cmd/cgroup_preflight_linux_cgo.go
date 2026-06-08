//go:build linux && cgo

package cmd

import (
	"github.com/datasance/edgelet/internal/cli/run"
	"github.com/spf13/cobra"
)

func newCgroupPreflightCommand() *cobra.Command {
	return &cobra.Command{
		Use:    "cgroup-preflight",
		Short:  "Validate cgroup mounts and delegation before start",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return run.NewCLIError(
				run.CodeInvalidArgument,
				"cgroup-preflight is provided by the edgelet thin binary; rebuild with CGO_ENABLED=0",
				nil,
			)
		},
	}
}
