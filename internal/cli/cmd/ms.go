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
		Use:     "ms",
		Short:   "Microservice operations",
		Long:    microservice.CommandLong(),
		Example: microservice.CommandExamples(),
	}

	cmd.AddCommand(
		newMSListCommand(),
		&cobra.Command{
			Use:    "ps",
			Hidden: true,
			RunE: func(cmd *cobra.Command, args []string) error {
				return run.NewCLIError(run.CodeInvalidArgument, "unknown ms subcommand \"ps\"; use \"iofog-agent ms ls\"", nil)
			},
		},
		newMSInspectCommand(),
		newMSLogsCommand(),
		&cobra.Command{
			Use:   "exec <id> [-- command...]",
			Short: "Execute a command in a microservice",
			Args:  cobra.MinimumNArgs(1),
			RunE:  runMSExec,
		},
		newMSLifecycleCommand("start", "Start a microservice", "", microservice.Start),
		newMSLifecycleCommand("stop", "Stop a microservice", "", microservice.Stop),
		newMSLifecycleCommand("restart", "Restart a microservice", "", microservice.Restart),
		newMSLifecycleCommand("kill", "Kill a microservice", microservice.KillCommandLong(), microservice.Kill),
		newMSLifecycleCommand("rm", "Remove a microservice", microservice.RemoveCommandLong(), microservice.Remove),
	)

	return cmd
}

func newMSListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ls",
		Short: "List microservices",
		Args:  cobra.NoArgs,
		RunE:  runMSList,
	}
	cmd.Flags().String("source", "all", "Filter list: managed, local, or all")
	return cmd
}

func newMSInspectCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "inspect <id>",
		Short: "Inspect a microservice",
		Args:  cobra.ExactArgs(1),
		RunE:  runMSInspect,
	}
	cmd.Flags().Bool("summary", false, "Show summary output")
	return cmd
}

func newMSLogsCommand() *cobra.Command {
	var flags logsFlagValues
	cmd := &cobra.Command{
		Use:   "logs <id>",
		Short: "Stream microservice logs",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if appCtx == nil {
				return run.NewCLIError(run.CodeInternal, "cli context is nil", nil)
			}
			if err := run.RequireDaemon(appCtx.Client); err != nil {
				return err
			}
			opts := flags.options()
			id := args[0]
			if opts.Follow {
				return microservice.StreamLogs(appCtx, concreteClient(appCtx), id, opts)
			}
			return microservice.FetchLogs(appCtx, appCtx.Client, id, opts)
		},
	}
	registerLogsFlags(cmd, &flags)
	return cmd
}

type msLifecycleFn func(run.V3Client, string) (*microservice.LifecycleResult, error)

func newMSLifecycleCommand(name, short, long string, fn msLifecycleFn) *cobra.Command {
	return &cobra.Command{
		Use:   name + " <id>",
		Short: short,
		Long:  long,
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
	source, err := cmd.Flags().GetString("source")
	if err != nil {
		return run.NewCLIError(run.CodeInternal, err.Error(), err)
	}
	source = strings.ToLower(strings.TrimSpace(source))
	if source == "" {
		source = "all"
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
	summary, err := cmd.Flags().GetBool("summary")
	if err != nil {
		return run.NewCLIError(run.CodeInternal, err.Error(), err)
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
