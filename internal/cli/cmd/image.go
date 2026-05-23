package cmd

import (
	"strings"

	"github.com/eclipse-iofog/agent/internal/cli/domain/image"
	"github.com/eclipse-iofog/agent/internal/cli/output"
	"github.com/eclipse-iofog/agent/internal/cli/run"
	"github.com/spf13/cobra"
)

func newImageCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "image",
		Short: "Image operations",
	}

	var imagePruneMode string

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
		func() *cobra.Command {
			pruneCmd := &cobra.Command{
				Use:       "prune [dangling]",
				Short:     "Prune dangling images",
				Long:      "Prune dangling images only.",
				Args:      cobra.MaximumNArgs(1),
				ValidArgs: []string{"dangling"},
				Example: strings.Join([]string{
					"iofog-agent image prune",
					"iofog-agent image prune dangling",
					"iofog-agent image prune --mode dangling",
				}, "\n"),
				RunE: runImagePrune,
			}
			pruneCmd.Flags().StringVarP(&imagePruneMode, "mode", "m", "", "Prune mode (only: dangling)")
			return pruneCmd
		}(),
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
	modeArg, _ := cmd.Flags().GetString("mode")
	parseArgs := make([]string, 0, len(args)+2)
	if strings.TrimSpace(modeArg) != "" {
		parseArgs = append(parseArgs, "--mode", modeArg)
	}
	parseArgs = append(parseArgs, args...)

	if appCtx.Format.IsStructured() {
		result, err := image.Prune(appCtx.Client, parseArgs)
		if err != nil {
			return err
		}
		return run.WriteValue(appCtx, result.Data)
	}

	spin := appCtx.UI.StartSpinner("Pruning dangling images...")
	result, err := image.Prune(appCtx.Client, parseArgs)
	spin.Stop()
	if err != nil {
		return err
	}
	human := strings.TrimSpace(result.Human)
	if human == "" {
		human = strings.TrimSpace(output.FormatV3Human("/v3/images:prune", result.Data))
	}
	if human == "" {
		return writeHumanOrRoute(appCtx, "/v3/images:prune", result.Human, result.Data)
	}
	return run.WriteHumanSuccess(appCtx, human)
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
