package cmd

import (
	"context"
	"strings"

	"github.com/eclipse-iofog/agent/internal/cli/domain/deploy"
	"github.com/eclipse-iofog/agent/internal/cli/domain/image"
	"github.com/eclipse-iofog/agent/internal/cli/run"
	"github.com/eclipse-iofog/agent/internal/cli/ui"
	"github.com/spf13/cobra"
)

func newDeployCommand() *cobra.Command {
	var (
		manifestPath string
		sourceName   string
		dryRun       bool
	)

	cmd := &cobra.Command{
		Use:     "deploy",
		Short:   "Deploy a local manifest",
		Long:    deploy.CommandLong(),
		Example: deploy.CommandExamples(),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				switch args[0] {
				case "apply", "validate", "registry", "registries", "runtimeclass", "runtimeclasses":
					return run.NewCLIError(run.CodeInvalidArgument, "unknown deploy arguments; use: iofog-agent deploy -f <manifest.yaml> [--dry-run]", nil)
				default:
					return run.NewCLIError(run.CodeInvalidArgument, "unknown deploy argument "+args[0], nil)
				}
			}
			if manifestPath == "" {
				return run.NewCLIError(run.CodeInvalidArgument, "usage: iofog-agent deploy -f <manifest.yaml> [--sourceName <name>] [--dry-run]", nil)
			}
			if appCtx == nil {
				return run.NewCLIError(run.CodeInternal, "cli context is nil", nil)
			}
			if err := run.RequireDaemon(appCtx.Client); err != nil {
				return err
			}
			api := appCtx.Client
			var uiProgress *ui.UI
			if !appCtx.Format.IsStructured() {
				uiProgress = appCtx.UI
			}
			result, err := deploy.Execute(context.Background(), api, uiProgress, deploy.Request{
				ManifestPath: manifestPath,
				SourceName:   sourceName,
				DryRun:       dryRun,
			})
			if err != nil {
				return err
			}
			if appCtx.Format.IsStructured() {
				return run.WriteValue(appCtx, result.Data)
			}
			return run.WriteHumanSuccess(appCtx, result.Human)
		},
	}

	cmd.Flags().StringVarP(&manifestPath, "file", "f", "", "Path to manifest YAML")
	cmd.Flags().StringVar(&sourceName, "sourceName", "", "Optional source name for microservice deploy")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate manifest without applying")
	_ = cmd.MarkFlagRequired("file")

	return cmd
}

func newImagePullCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pull <image-ref>",
		Short: "Pull an image",
		Args:  cobra.ExactArgs(1),
		RunE:  runImagePull,
	}
	cmd.Flags().IntP("registry-id", "r", 0, "Registry ID")
	cmd.Flags().StringP("platform", "p", "", "Platform os/arch[/variant]")
	return cmd
}

func runImagePull(cmd *cobra.Command, args []string) error {
	if appCtx == nil {
		return run.NewCLIError(run.CodeInternal, "cli context is nil", nil)
	}
	if err := run.RequireDaemon(appCtx.Client); err != nil {
		return err
	}
	registryID, _ := cmd.Flags().GetInt("registry-id")
	platform, _ := cmd.Flags().GetString("platform")
	var uiProgress *ui.UI
	if !appCtx.Format.IsStructured() {
		uiProgress = appCtx.UI
	}
	api := appCtx.Client
	result, err := image.Pull(context.Background(), api, uiProgress, image.PullRequest{
		Image:      strings.TrimSpace(args[0]),
		RegistryID: registryID,
		Platform:   strings.TrimSpace(platform),
	})
	if err != nil {
		return err
	}
	if appCtx.Format.IsStructured() {
		return run.WriteValue(appCtx, result.Data)
	}
	return run.WriteHumanSuccess(appCtx, result.Human)
}
