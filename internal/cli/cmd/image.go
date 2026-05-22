package cmd

import (
	"github.com/eclipse-iofog/agent/internal/cli/domain/image"
	"github.com/eclipse-iofog/agent/internal/cli/run"
	"github.com/spf13/cobra"
)

func newImageCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "image",
		Short: "Image operations",
	}

	cmd.AddCommand(
		&cobra.Command{
			Use:   "ls",
			Short: "List images",
			RunE:  runGET("/v3/images"),
		},
		newImagePullCommand(),
		&cobra.Command{
			Use:   "load",
			Short: "Load an image archive",
			RunE:  runImageLoad,
		},
		&cobra.Command{
			Use:   "prune",
			Short: "Prune dangling images",
			RunE:  runImagePrune,
		},
		&cobra.Command{
			Use:   "rm",
			Short: "Remove an image",
			Args:  cobra.ExactArgs(1),
			RunE:  runImageRemove,
		},
	)

	return cmd
}

func runImageLoad(cmd *cobra.Command, args []string) error {
	if appCtx == nil {
		return run.NewCLIError(run.CodeInternal, "cli context is nil", nil)
	}
	if err := run.RequireDaemon(appCtx.Client); err != nil {
		return err
	}
	req, err := image.ParseLoadArgs(args)
	if err != nil {
		return err
	}
	result, err := image.Load(appCtx.Client, *req)
	if err != nil {
		return err
	}
	return writeHumanOrRoute(appCtx, "/v3/images:load", result.Human, result.Data)
}

func runImagePrune(cmd *cobra.Command, args []string) error {
	if appCtx == nil {
		return run.NewCLIError(run.CodeInternal, "cli context is nil", nil)
	}
	if err := run.RequireDaemon(appCtx.Client); err != nil {
		return err
	}
	result, err := image.Prune(appCtx.Client, args)
	if err != nil {
		return err
	}
	return writeHumanOrRoute(appCtx, "/v3/images:prune", result.Human, result.Data)
}

func runImageRemove(cmd *cobra.Command, args []string) error {
	if appCtx == nil {
		return run.NewCLIError(run.CodeInternal, "cli context is nil", nil)
	}
	if err := run.RequireDaemon(appCtx.Client); err != nil {
		return err
	}
	result, err := image.Remove(appCtx.Client, args[0])
	if err != nil {
		return err
	}
	return writeHumanOrRoute(appCtx, "/v3/images:remove", result.Human, result.Data)
}
