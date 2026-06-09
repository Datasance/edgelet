//go:build linux && !cgo

package cmd

import (
	"fmt"
	"os"

	"github.com/eclipse-iofog/edgelet/internal/cgroups"
	"github.com/eclipse-iofog/edgelet/internal/cli/run"
	"github.com/spf13/cobra"
)

func newCgroupPreflightCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "cgroup-preflight",
		Short: "Validate cgroup mounts and delegation before start",
		Long:  "Runs cgroup detect + light preflight checks used by init start_pre hooks. Does not mutate cgroups.",
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
		_, _ = fmt.Fprintf(os.Stderr, "WARN: %s\n", policy.HybridWarning)
	}
	if err := cgroups.ValidatePreflightLight(policy); err != nil {
		return run.NewCLIError(run.CodeInternal, fmt.Sprintf("cgroup preflight failed: %v", err), err)
	}
	return nil
}
