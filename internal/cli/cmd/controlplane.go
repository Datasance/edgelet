package cmd

import (
	"github.com/datasance/edgelet/internal/cli/domain/controlplane"
	"github.com/datasance/edgelet/internal/cli/run"
	"github.com/spf13/cobra"
)

func newControlPlaneCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "controlplane",
		Short:   "Control plane controller operations",
		Long:    controlplane.CommandLong(),
		Example: controlplane.CommandExamples(),
	}

	cmd.AddCommand(newControlPlaneGetCommand(), newControlPlaneDeleteCommand())
	return cmd
}

func newControlPlaneGetCommand() *cobra.Command {
	var manifest bool
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Show control plane deployment status or masked manifest",
		RunE: func(cmd *cobra.Command, args []string) error {
			if appCtx == nil {
				return run.NewCLIError(run.CodeInternal, "cli context is nil", nil)
			}
			if err := run.RequireDaemon(appCtx.Client); err != nil {
				return err
			}
			path := "/v1/system/controlplane"
			if manifest {
				path = "/v1/system/controlplane/manifest"
			}
			data, err := appCtx.Client.Request("GET", path, nil)
			if err != nil {
				return run.MapAPIError(err)
			}
			return run.WriteRouteData(appCtx, path, data)
		},
	}
	cmd.Flags().BoolVar(&manifest, "manifest", false, "Return secrets-masked ControlPlane manifest YAML")
	registerControlPlaneManifestCompletion(cmd)
	return cmd
}

func newControlPlaneDeleteCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "delete",
		Short: "Remove the control plane deployment",
		RunE:  runControlPlaneDelete,
	}
}

func runControlPlaneDelete(cmd *cobra.Command, args []string) error {
	if appCtx == nil {
		return run.NewCLIError(run.CodeInternal, "cli context is nil", nil)
	}
	if err := run.RequireDaemon(appCtx.Client); err != nil {
		return err
	}
	path := "/v1/system/controlplane"
	var data map[string]any
	err := run.WithSpinner(appCtx, "Removing control plane deployment...", func() error {
		var reqErr error
		data, reqErr = appCtx.Client.Request("DELETE", path, nil)
		return run.MapAPIError(reqErr)
	})
	if err != nil {
		return err
	}
	return run.WriteRouteData(appCtx, path, data)
}

func registerControlPlaneManifestCompletion(cmd *cobra.Command) {
	_ = cmd.RegisterFlagCompletionFunc("manifest", func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return []string{"true", "false"}, cobra.ShellCompDirectiveNoFileComp
	})
}
