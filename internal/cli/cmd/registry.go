package cmd

import (
	"github.com/eclipse-iofog/agent/internal/cli/domain/registry"
	"github.com/eclipse-iofog/agent/internal/cli/run"
	"github.com/spf13/cobra"
)

func newRegistryCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "registry",
		Short:   "Registry operations",
		Long:    registry.CommandLong(),
		Example: registry.CommandExamples(),
	}

	cmd.AddCommand(
		&cobra.Command{
			Use:   "ls",
			Short: "List registries",
			RunE:  runGET("/v3/deploy/registries"),
		},
		newRegistryInspectCommand(),
		&cobra.Command{
			Use:   "rm <id>",
			Short: "Remove a registry",
			Args:  cobra.ExactArgs(1),
			RunE:  runRegistryRemove,
		},
	)

	return cmd
}

func newRegistryInspectCommand() *cobra.Command {
	var passwordPlain bool
	cmd := &cobra.Command{
		Use:   "inspect <id>",
		Short: "Inspect a registry",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if appCtx == nil {
				return run.NewCLIError(run.CodeInternal, "cli context is nil", nil)
			}
			if err := run.RequireDaemon(appCtx.Client); err != nil {
				return err
			}
			result, err := registry.Inspect(appCtx.Client, args[0], passwordPlain)
			if err != nil {
				return err
			}
			if appCtx.Format.IsStructured() {
				return run.WriteValue(appCtx, result.Data)
			}
			return run.WriteHuman(appCtx, result.Human)
		},
	}
	cmd.Flags().BoolVar(&passwordPlain, "password-plain", false, "Show registry password in plain text")
	return cmd
}

func runRegistryRemove(cmd *cobra.Command, args []string) error {
	if appCtx == nil {
		return run.NewCLIError(run.CodeInternal, "cli context is nil", nil)
	}
	if err := run.RequireDaemon(appCtx.Client); err != nil {
		return err
	}
	path := "/v3/deploy/registries/" + args[0]
	data, err := appCtx.Client.RequestV3("DELETE", path, nil)
	if err != nil {
		return run.MapAPIError(err)
	}
	return run.WriteRouteData(appCtx, path, data)
}
