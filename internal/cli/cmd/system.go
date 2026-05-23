package cmd

import (
	"strings"

	"github.com/eclipse-iofog/agent/internal/cli/domain/prune"
	"github.com/eclipse-iofog/agent/internal/cli/domain/system"
	"github.com/eclipse-iofog/agent/internal/cli/output"
	"github.com/eclipse-iofog/agent/internal/cli/run"
	"github.com/spf13/cobra"
)

func newSystemCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "system",
		Short: "System operations",
	}

	var systemPruneMode string

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
		func() *cobra.Command {
			pruneCmd := &cobra.Command{
				Use:       "prune [dangling|containers|volumes|all]",
				Short:     "Prune unused resources",
				Long:      "Prune unused resources. Default mode is dangling images.",
				Args:      cobra.MaximumNArgs(1),
				ValidArgs: []string{"dangling", "containers", "volumes", "all"},
				Example: strings.Join([]string{
					"iofog-agent system prune",
					"iofog-agent system prune all",
					"iofog-agent system prune --mode all",
					"iofog-agent system prune --mode volumes",
				}, "\n"),
				RunE: runSystemPrune,
			}
			pruneCmd.Flags().StringVarP(&systemPruneMode, "mode", "m", "", "Prune mode: dangling|containers|volumes|all")
			return pruneCmd
		}(),
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
	modeArg, _ := cmd.Flags().GetString("mode")
	parseArgs := make([]string, 0, len(args)+2)
	if strings.TrimSpace(modeArg) != "" {
		parseArgs = append(parseArgs, "--mode", modeArg)
	}
	parseArgs = append(parseArgs, args...)
	mode, err := prune.ParseMode(parseArgs, "Usage: iofog-agent system prune [dangling|containers|volumes|all]")
	if err != nil {
		return err
	}
	path := "/v3/system/prune"
	if mode != "" {
		path += "?mode=" + mode
	}
	if appCtx.Format.IsStructured() {
		data, reqErr := appCtx.Client.RequestV3("POST", path, nil)
		if reqErr != nil {
			return run.MapAPIError(reqErr)
		}
		return run.WriteRouteData(appCtx, path, data)
	}
	spin := appCtx.UI.StartSpinner("Pruning resources...")
	data, err := appCtx.Client.RequestV3("POST", path, nil)
	spin.Stop()
	if err != nil {
		return run.MapAPIError(err)
	}
	human := output.FormatV3Human(path, data)
	if strings.TrimSpace(human) == "" {
		return run.WriteRouteData(appCtx, path, data)
	}
	return run.WriteHumanSuccess(appCtx, human)
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
