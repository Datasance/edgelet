package cmd

import (
	"context"
	"strings"

	"github.com/eclipse-iofog/edgelet/internal/cli/domain/image"
	"github.com/eclipse-iofog/edgelet/internal/cli/output"
	"github.com/eclipse-iofog/edgelet/internal/cli/run"
	"github.com/eclipse-iofog/edgelet/internal/cli/ui"
	"github.com/spf13/cobra"
)

func newImageCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "image",
		Short:   "Image operations",
		Long:    image.CommandLong(),
		Example: image.CommandExamples(),
	}

	var imagePruneMode string

	cmd.AddCommand(
		&cobra.Command{
			Use:   "ls",
			Short: "List images",
			RunE:  runGET("/v1/images"),
		},
		newImagePullCommand(),
		newImageLoadCommand(),
		func() *cobra.Command {
			pruneCmd := &cobra.Command{
				Use:       "prune [dangling]",
				Short:     "Prune dangling images",
				Long:      "Prune dangling images only.",
				Args:      cobra.MaximumNArgs(1),
				ValidArgs: []string{"dangling"},
				Example: strings.Join([]string{
					"edgelet image prune",
					"edgelet image prune dangling",
					"edgelet image prune --mode dangling",
				}, "\n"),
				RunE: runImagePrune,
			}
			pruneCmd.Flags().StringVarP(&imagePruneMode, "mode", "m", "", "Prune mode (only: dangling)")
			registerImagePruneModeCompletion(pruneCmd)
			return pruneCmd
		}(),
		&cobra.Command{
			Use:   "rm <selector>",
			Short: "Remove an image",
			Args:  cobra.ExactArgs(1),
			RunE:  runImageRemove,
		},
	)

	return cmd
}

func newImageLoadCommand() *cobra.Command {
	var filePath string
	cmd := &cobra.Command{
		Use:   "load",
		Short: "Load an image archive",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if appCtx == nil {
				return run.NewCLIError(run.CodeInternal, "cli context is nil", nil)
			}
			if err := run.RequireDaemon(appCtx.Client); err != nil {
				return err
			}
			var uiProgress *ui.UI
			if !appCtx.Format.IsStructured() {
				uiProgress = appCtx.UI
			}
			result, err := image.Load(context.Background(), appCtx.Client, uiProgress, image.LoadRequest{Path: filePath})
			if err != nil {
				return err
			}
			return writeHumanOrRoute(appCtx, "/v1/images:load", result.Human, result.Data)
		},
	}
	cmd.Flags().StringVarP(&filePath, "file", "f", "", "Path to image tar or tar.gz archive")
	_ = cmd.MarkFlagRequired("file")
	return cmd
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
		human = strings.TrimSpace(output.FormatEdgeletAPIHuman("/v1/images:prune", result.Data))
	}
	if human == "" {
		return writeHumanOrRoute(appCtx, "/v1/images:prune", result.Human, result.Data)
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
	selector := args[0]
	var result *image.RemoveResult
	err := run.WithSpinner(appCtx, "Removing image "+selector+"...", func() error {
		var err error
		result, err = image.Remove(appCtx.Client, selector)
		return err
	})
	if err != nil {
		return err
	}
	return writeHumanOrRoute(appCtx, "/v1/images:remove", result.Human, result.Data)
}
