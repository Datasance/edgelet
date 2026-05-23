package cmd

import (
	"github.com/eclipse-iofog/agent/internal/cli/domain/provision"
	"github.com/eclipse-iofog/agent/internal/cli/run"
	"github.com/spf13/cobra"
)

func newProvisionCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "provision <provisioning-key>",
		Short:   "Provision the agent",
		Long:    provision.ProvisionLong(),
		Example: provision.ProvisionExamples(),
		Args:    cobra.ExactArgs(1),
		RunE:    runProvision,
	}
}

func newDeprovisionCommand() *cobra.Command {
	var scope string
	var keepLocal bool

	cmd := &cobra.Command{
		Use:     "deprovision",
		Short:   "Deprovision the agent",
		Long:    provision.DeprovisionLong(),
		Example: provision.DeprovisionExamples(),
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if appCtx == nil {
				return run.NewCLIError(run.CodeInternal, "cli context is nil", nil)
			}
			if err := run.RequireDaemon(appCtx.Client); err != nil {
				return err
			}
			scopeVal := scope
			if keepLocal {
				scopeVal = "local"
			}
			result, err := provision.Deprovision(appCtx.Client, provision.DeprovisionRequest{Scope: scopeVal})
			if err != nil {
				return err
			}
			return writeHumanOrData(appCtx, result.Human, result.Data)
		},
	}
	cmd.Flags().StringVar(&scope, "scope", "all", "Deprovision scope: all or local")
	cmd.Flags().BoolVar(&keepLocal, "keep-local", false, "Preserve local microservices (sets scope to local)")
	return cmd
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
