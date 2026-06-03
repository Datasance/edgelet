//go:build linux && !cgo

package cmd

import (
	"fmt"
	"os"

	"github.com/datasance/edgelet/internal/cgroups"
	"github.com/datasance/edgelet/internal/cli/run"
	"github.com/spf13/cobra"
)

func newCgroupPreflightCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "cgroup-preflight",
		Short: "Validate cgroup mounts and delegation before start",
		Long:  "Runs cgroup detect + preflight checks used by init start_pre hooks. Does not mutate cgroups.",
		Args:  cobra.NoArgs,
		RunE:  runCgroupPreflight,
	}
}

func runCgroupPreflight(cmd *cobra.Command, args []string) error {
	policy, err := cgroups.DetectPreflight()
	if err != nil {
		return run.NewCLIError(run.CodeInternal, fmt.Sprintf("cgroup detect failed: %v", err), err)
	}
	if policy.HybridWarning != "" {
		fmt.Fprintf(os.Stderr, "WARN: %s\n", policy.HybridWarning)
	}
	if err := cgroups.ValidatePreflight(policy); err != nil {
		return run.NewCLIError(run.CodeInternal, fmt.Sprintf("cgroup preflight failed: %v", err), err)
	}
	return nil
}
