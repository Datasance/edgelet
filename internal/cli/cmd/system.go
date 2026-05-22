package cmd

import (
	"github.com/eclipse-iofog/agent/internal/cli/domain/prune"
	"github.com/eclipse-iofog/agent/internal/cli/domain/system"
	"github.com/eclipse-iofog/agent/internal/cli/run"
	"github.com/spf13/cobra"
)

func newSystemCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "system",
		Short: "System operations",
	}

	cmd.AddCommand(
		&cobra.Command{
			Use:   "status",
			Short: "Show agent runtime status",
			RunE:  runGET("/v3/system/status"),
		},
		&cobra.Command{
			Use:   "info",
			Short: "Show agent configuration info",
			RunE:  runGET("/v3/system/info"),
		},
		&cobra.Command{
			Use:   "version",
			Short: "Show combined CLI and daemon version",
			RunE: func(cmd *cobra.Command, args []string) error {
				return runVersion(appCtx)
			},
		},
		&cobra.Command{
			Use:   "reload",
			Short: "Reload daemon configuration",
			RunE:  runPOST("/v3/system/reload", nil),
		},
		&cobra.Command{
			Use:   "stop",
			Short: "Gracefully stop the daemon",
			RunE:  runSystemStop,
		},
		&cobra.Command{
			Use:   "prune",
			Short: "Prune unused resources",
			RunE:  runSystemPrune,
		},
		&cobra.Command{
			Use:   "logs",
			Short: "Stream daemon logs",
			RunE:  runSystemLogs,
		},
	)

	return cmd
}

func runSystemStop(cmd *cobra.Command, args []string) error {
	if appCtx == nil {
		return run.NewCLIError(run.CodeInternal, "cli context is nil", nil)
	}
	if err := run.RequireDaemon(appCtx.Client); err != nil {
		return err
	}
	result, err := system.Stop(appCtx.Client)
	if err != nil {
		return err
	}
	return writeHumanOrRoute(appCtx, "/v3/system/stop", result.Human, result.Data)
}

func runSystemPrune(cmd *cobra.Command, args []string) error {
	if appCtx == nil {
		return run.NewCLIError(run.CodeInternal, "cli context is nil", nil)
	}
	if err := run.RequireDaemon(appCtx.Client); err != nil {
		return err
	}
	mode, err := prune.ParseMode(args, "Usage: iofog-agent system prune [dangling|containers|volumes|all]")
	if err != nil {
		return err
	}
	path := "/v3/system/prune"
	if mode != "" {
		path += "?mode=" + mode
	}
	data, err := appCtx.Client.RequestV3("POST", path, nil)
	if err != nil {
		return run.MapAPIError(err)
	}
	return run.WriteRouteData(appCtx, path, data)
}

func runSystemLogs(cmd *cobra.Command, args []string) error {
	if appCtx == nil {
		return run.NewCLIError(run.CodeInternal, "cli context is nil", nil)
	}
	if err := run.RequireDaemon(appCtx.Client); err != nil {
		return err
	}
	opts, err := system.ParseLogsOptions(args)
	if err != nil {
		return err
	}
	if opts.Follow {
		return system.StreamLogs(appCtx, concreteClient(appCtx), *opts)
	}
	return system.FetchLogs(appCtx, appCtx.Client, *opts)
}

func runGET(path string) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		if appCtx == nil {
			return run.NewCLIError(run.CodeInternal, "cli context is nil", nil)
		}
		if err := run.RequireDaemon(appCtx.Client); err != nil {
			return err
		}
		data, err := appCtx.Client.RequestV3("GET", path, nil)
		if err != nil {
			return run.MapAPIError(err)
		}
		return run.WriteRouteData(appCtx, path, data)
	}
}

func runPOST(path string, payload any) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		if appCtx == nil {
			return run.NewCLIError(run.CodeInternal, "cli context is nil", nil)
		}
		if err := run.RequireDaemon(appCtx.Client); err != nil {
			return err
		}
		data, err := appCtx.Client.RequestV3("POST", path, payload)
		if err != nil {
			return run.MapAPIError(err)
		}
		return run.WriteRouteData(appCtx, path, data)
	}
}
