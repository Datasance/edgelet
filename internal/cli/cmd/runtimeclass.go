package cmd

import (
	"github.com/eclipse-iofog/agent/internal/cli/run"
	"github.com/spf13/cobra"
)

func newRuntimeClassCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "runtimeclass",
		Short: "Runtime class operations",
	}

	cmd.AddCommand(
		&cobra.Command{
			Use:   "ls",
			Short: "List runtime classes",
			RunE:  runGET("/v3/deploy/runtimeclasses"),
		},
		&cobra.Command{
			Use:   "inspect",
			Short: "Inspect a runtime class",
			Args:  cobra.ExactArgs(1),
			RunE:  runRuntimeClassInspect,
		},
		&cobra.Command{
			Use:   "rm",
			Short: "Remove a runtime class",
			Args:  cobra.ExactArgs(1),
			RunE:  runRuntimeClassRemove,
		},
	)

	return cmd
}

func runRuntimeClassInspect(cmd *cobra.Command, args []string) error {
	if appCtx == nil {
		return run.NewCLIError(run.CodeInternal, "cli context is nil", nil)
	}
	if err := run.RequireDaemon(appCtx.Client); err != nil {
		return err
	}
	path := "/v3/deploy/runtimeclasses/" + args[0]
	data, err := appCtx.Client.RequestV3("GET", path, nil)
	if err != nil {
		return run.MapAPIError(err)
	}
	return run.WriteRouteData(appCtx, path, data)
}

func runRuntimeClassRemove(cmd *cobra.Command, args []string) error {
	if appCtx == nil {
		return run.NewCLIError(run.CodeInternal, "cli context is nil", nil)
	}
	if err := run.RequireDaemon(appCtx.Client); err != nil {
		return err
	}
	path := "/v3/deploy/runtimeclasses/" + args[0]
	data, err := appCtx.Client.RequestV3("DELETE", path, nil)
	if err != nil {
		return run.MapAPIError(err)
	}
	return run.WriteRouteData(appCtx, path, data)
}
