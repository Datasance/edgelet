package cmd

import (
	"github.com/datasance/edgelet/internal/cli/domain/registry"
	"github.com/datasance/edgelet/internal/cli/run"
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
			RunE:  runGET("/v1/deploy/registries"),
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
	registerRegistryInspectCompletions(cmd)
	return cmd
}

func registerRegistryInspectCompletions(cmd *cobra.Command) {
	_ = cmd.RegisterFlagCompletionFunc("password-plain", func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return []string{"true", "false"}, cobra.ShellCompDirectiveNoFileComp
	})
}

func runRegistryRemove(cmd *cobra.Command, args []string) error {
	if appCtx == nil {
		return run.NewCLIError(run.CodeInternal, "cli context is nil", nil)
	}
	if err := run.RequireDaemon(appCtx.Client); err != nil {
		return err
	}
	path := "/v1/deploy/registries/" + args[0]
	var data map[string]interface{}
	err := run.WithSpinner(appCtx, "Removing registry "+args[0]+"...", func() error {
		var reqErr error
		data, reqErr = appCtx.Client.RequestV3("DELETE", path, nil)
		return run.MapAPIError(reqErr)
	})
	if err != nil {
		return err
	}
	return run.WriteRouteData(appCtx, path, data)
}
