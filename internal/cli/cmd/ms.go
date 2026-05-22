package cmd

import (
	"strings"

	"github.com/eclipse-iofog/agent/internal/cli/client"
	"github.com/eclipse-iofog/agent/internal/cli/domain/microservice"
	"github.com/eclipse-iofog/agent/internal/cli/run"
	"github.com/spf13/cobra"
)

func newMSCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ms",
		Short: "Microservice operations",
	}

	cmd.AddCommand(
		&cobra.Command{
			Use:   "ls",
			Short: "List microservices",
			RunE:  runMSList,
		},
		&cobra.Command{
			Use:    "ps",
			Hidden: true,
			RunE: func(cmd *cobra.Command, args []string) error {
				return run.NewCLIError(run.CodeInvalidArgument, "unknown ms subcommand \"ps\"; use \"iofog-agent ms ls\"", nil)
			},
		},
		&cobra.Command{
			Use:   "inspect",
			Short: "Inspect a microservice",
			RunE:  runMSInspect,
		},
		&cobra.Command{
			Use:   "logs",
			Short: "Stream microservice logs",
			RunE:  runMSLogs,
		},
		&cobra.Command{
			Use:   "exec",
			Short: "Execute a command in a microservice",
			RunE:  runMSExec,
		},
		newMSLifecycleCommand("start", "Start a microservice", microservice.Start),
		newMSLifecycleCommand("stop", "Stop a microservice", microservice.Stop),
		newMSLifecycleCommand("restart", "Restart a microservice", microservice.Restart),
		newMSLifecycleCommand("kill", "Kill a microservice", microservice.Kill),
		newMSLifecycleCommand("rm", "Remove a microservice", microservice.Remove),
	)

	return cmd
}

type msLifecycleFn func(run.V3Client, string) (*microservice.LifecycleResult, error)

func newMSLifecycleCommand(name, short string, fn msLifecycleFn) *cobra.Command {
	return &cobra.Command{
		Use:   name + " <id>",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if appCtx == nil {
				return run.NewCLIError(run.CodeInternal, "cli context is nil", nil)
			}
			if err := run.RequireDaemon(appCtx.Client); err != nil {
				return err
			}
			result, err := fn(appCtx.Client, args[0])
			if err != nil {
				return err
			}
			return writeHumanOrRoute(appCtx, result.Path, result.Human, result.Data)
		},
	}
}

func runMSList(cmd *cobra.Command, args []string) error {
	if appCtx == nil {
		return run.NewCLIError(run.CodeInternal, "cli context is nil", nil)
	}
	if err := run.RequireDaemon(appCtx.Client); err != nil {
		return err
	}
	source := "all"
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--source":
			if i+1 >= len(args) {
				return run.NewCLIError(run.CodeInvalidArgument, "--source requires managed|local|all", nil)
			}
			source = strings.ToLower(strings.TrimSpace(args[i+1]))
			i++
		default:
			return run.NewCLIError(run.CodeInvalidArgument, "unknown flag "+args[i], nil)
		}
	}
	if source != "managed" && source != "local" && source != "all" {
		return run.NewCLIError(run.CodeInvalidArgument, "--source requires managed|local|all", nil)
	}
	path := "/v3/ms?source=" + source
	data, err := appCtx.Client.RequestV3("GET", path, nil)
	if err != nil {
		return run.MapAPIError(err)
	}
	return run.WriteRouteData(appCtx, "/v3/ms", data)
}

func runMSInspect(cmd *cobra.Command, args []string) error {
	if appCtx == nil {
		return run.NewCLIError(run.CodeInternal, "cli context is nil", nil)
	}
	if err := run.RequireDaemon(appCtx.Client); err != nil {
		return err
	}
	if len(args) < 1 {
		return run.NewCLIError(run.CodeInvalidArgument, "usage: iofog-agent ms inspect <id> [--summary]", nil)
	}
	summary := false
	for i := 1; i < len(args); i++ {
		if args[i] == "--summary" {
			summary = true
			continue
		}
		return run.NewCLIError(run.CodeInvalidArgument, "unknown flag "+args[i], nil)
	}
	path := "/v3/ms/" + args[0]
	if summary {
		path += "?summary=true"
	}
	data, err := appCtx.Client.RequestV3("GET", path, nil)
	if err != nil {
		return run.MapAPIError(err)
	}
	return run.WriteRouteData(appCtx, path, data)
}

func runMSLogs(cmd *cobra.Command, args []string) error {
	if appCtx == nil {
		return run.NewCLIError(run.CodeInternal, "cli context is nil", nil)
	}
	if err := run.RequireDaemon(appCtx.Client); err != nil {
		return err
	}
	if len(args) < 1 {
		return run.NewCLIError(run.CodeInvalidArgument, "usage: iofog-agent ms logs <id> [--follow] [--tail N] [--since ISO8601] [--until ISO8601] [--timestamps]", nil)
	}
	id := args[0]
	opts, err := microservice.ParseLogsOptions(args[1:])
	if err != nil {
		return err
	}
	if opts.Follow {
		return microservice.StreamLogs(appCtx, concreteClient(appCtx), id, *opts)
	}
	return microservice.FetchLogs(appCtx, appCtx.Client, id, *opts)
}

func runMSExec(cmd *cobra.Command, args []string) error {
	if appCtx == nil {
		return run.NewCLIError(run.CodeInternal, "cli context is nil", nil)
	}
	if err := run.RequireDaemon(appCtx.Client); err != nil {
		return err
	}
	if appCtx.Format.IsStructured() {
		return run.NewCLIError(run.CodeInvalidArgument, "exec output supports human format only", nil)
	}
	id, command, err := microservice.ParseExecArgs(args)
	if err != nil {
		return err
	}
	return microservice.Exec(concreteClient(appCtx), id, command)
}

func concreteClient(ctx *run.CLIContext) *client.Client {
	if ctx == nil {
		return client.New()
	}
	if c, ok := ctx.Client.(*client.Client); ok {
		return c
	}
	return client.New()
}
