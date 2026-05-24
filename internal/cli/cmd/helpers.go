package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/datasance/edgelet/internal/cli/output"
	"github.com/datasance/edgelet/internal/cli/run"
	"github.com/datasance/edgelet/internal/cli/ui"
	"github.com/spf13/cobra"
)

func printBannerTo(errOut io.Writer) {
	if errOut == nil {
		errOut = os.Stderr
	}
	fmt.Fprint(errOut, banner)
	fmt.Fprintf(errOut, "  Version: %s\n\n", Version)
}

func filterHelpArgs(args []string) []string {
	if len(args) == 0 {
		return nil
	}
	filtered := make([]string, 0, len(args))
	for _, arg := range args {
		switch arg {
		case "--help", "-h":
			continue
		default:
			filtered = append(filtered, arg)
		}
	}
	return filtered
}

func shouldPrintBanner(c *cobra.Command, args []string) bool {
	if c == nil {
		return false
	}
	if len(filterHelpArgs(args)) > 0 {
		return false
	}
	return c.Name() == "iofog-agent" || c.Name() == "help"
}

func writeCommandError(err error) {
	if err == nil {
		return
	}
	if errors.Is(err, run.ErrHumanOutputWritten) {
		return
	}
	msg := err.Error()
	if appCtx != nil && appCtx.UI != nil {
		if strings.HasPrefix(msg, "Error[") {
			if idx := strings.Index(msg, "]: "); idx >= 0 {
				appCtx.UI.WriteError(msg[idx+3:])
				return
			}
		}
		appCtx.UI.WriteError(msg)
		return
	}
	fmt.Fprintln(os.Stderr, msg)
}

func contextOrBootstrap() *run.CLIContext {
	if appCtx != nil {
		return appCtx
	}
	appCtx = run.NewCLIContext(ui.Options{}, output.FormatHuman)
	appCtx.Version = Version
	appCtx.BuildTime = BuildTime
	appCtx.GitCommit = GitCommit
	appCtx.Client = newClient()
	return appCtx
}

func runVersion(ctx *run.CLIContext) error {
	ctx = contextOrBootstrap()
	if err := run.RequireDaemon(ctx.Client); err != nil {
		return err
	}
	daemon, err := ctx.Client.RequestV3("GET", "/v1/system/version", nil)
	payload := output.BuildVersionPayload(ctx.Version, ctx.BuildTime, ctx.GitCommit, daemon, err)
	if ctx.Format.IsStructured() {
		return run.WriteValue(ctx, payload)
	}
	return run.WriteValue(ctx, output.FormatVersionHuman(ctx.Version, ctx.BuildTime, ctx.GitCommit, daemon, err))
}

func writeHumanOrData(ctx *run.CLIContext, human string, data map[string]interface{}) error {
	if ctx == nil {
		return run.NewCLIError(run.CodeInternal, "cli context is nil", nil)
	}
	if ctx.Format.IsStructured() {
		return run.WriteValue(ctx, data)
	}
	return run.WriteHuman(ctx, human)
}

func writeHumanOrRoute(ctx *run.CLIContext, routePath, human string, data map[string]interface{}) error {
	if ctx == nil {
		return run.NewCLIError(run.CodeInternal, "cli context is nil", nil)
	}
	if ctx.Format.IsStructured() {
		return run.WriteValue(ctx, data)
	}
	if human != "" {
		return run.WriteHuman(ctx, human)
	}
	return run.WriteRouteData(ctx, routePath, data)
}

func writeHumanMutationOrData(ctx *run.CLIContext, human string, data map[string]interface{}) error {
	if ctx == nil {
		return run.NewCLIError(run.CodeInternal, "cli context is nil", nil)
	}
	if ctx.Format.IsStructured() {
		return run.WriteValue(ctx, data)
	}
	if human != "" {
		return run.WriteHumanMutationResult(ctx, human)
	}
	return run.WriteValue(ctx, data)
}

func writeHumanMutationOrRoute(ctx *run.CLIContext, routePath, human string, data map[string]interface{}) error {
	if ctx == nil {
		return run.NewCLIError(run.CodeInternal, "cli context is nil", nil)
	}
	if ctx.Format.IsStructured() {
		return run.WriteValue(ctx, data)
	}
	if human != "" {
		return run.WriteHumanMutationResult(ctx, human)
	}
	return run.WriteRouteData(ctx, routePath, data)
}
