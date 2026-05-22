package cmd

import (
	"github.com/eclipse-iofog/agent/internal/cli/domain/provision"
	"github.com/eclipse-iofog/agent/internal/cli/run"
	"github.com/spf13/cobra"
)

func newProvisionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "provision <provisioning-key>",
		Short: "Provision the agent",
		Args:  cobra.ExactArgs(1),
		RunE:  runProvision,
	}
}

func newDeprovisionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "deprovision",
		Short: "Deprovision the agent",
		RunE:  runDeprovision,
	}
}

func runProvision(cmd *cobra.Command, args []string) error {
	if appCtx == nil {
		return run.NewCLIError(run.CodeInternal, "cli context is nil", nil)
	}
	if err := run.RequireDaemon(appCtx.Client); err != nil {
		return err
	}
	result, err := provision.Provision(appCtx.Client, args[0])
	if err != nil {
		return err
	}
	return writeHumanOrData(appCtx, result.Human, result.Data)
}

func runDeprovision(cmd *cobra.Command, args []string) error {
	if appCtx == nil {
		return run.NewCLIError(run.CodeInternal, "cli context is nil", nil)
	}
	if err := run.RequireDaemon(appCtx.Client); err != nil {
		return err
	}
	result, err := provision.Deprovision(appCtx.Client, args)
	if err != nil {
		return err
	}
	return writeHumanOrData(appCtx, result.Human, result.Data)
}
