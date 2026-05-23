package cmd

import (
	"strings"

	"github.com/eclipse-iofog/agent/internal/cli/output"
	"github.com/eclipse-iofog/agent/internal/cli/run"
	"github.com/eclipse-iofog/agent/internal/cli/ui"
	"github.com/spf13/cobra"
)

var (
	rootCmd   *cobra.Command
	appCtx    *run.CLIContext
	newClient = run.DefaultClientFactory
)

const banner = "\n" +
	"  _        __                                     _   \n" +
	" (_)      / _|                                   | |  \n" +
	"  _  ___ | |_ ___   __ _    __ _  __ _  ___ _ __ | |_ \n" +
	" | |/ _ \\|  _/ _ \\ / _` |  / _` |/ _` |/ _ \\ '_ \\| __|\n" +
	" | | (_) | || (_) | (_| | | (_| | (_| |  __/ | | | |_ \n" +
	" |_|\\___/|_| \\___/ \\__, |  \\__,_| \\__, |\\___|_| |_|\\__|\n" +
	"                    __/ |         __/ |               \n" +
	"                   |___/         |___/                \n\n" +
	"  Datasance PoT ioFog Agent\n" +
	"  Command Line Interface\n" +
	"  =====================\n"

// Execute runs the Cobra command tree and returns a process exit code.
func Execute() int {
	rootCmd = newRootCommand()
	if err := rootCmd.Execute(); err != nil {
		writeCommandError(err)
		return run.ExitCodeForError(err)
	}
	return run.ExitSuccess
}

func newRootCommand() *cobra.Command {
	var (
		outputFormat string
		quiet        bool
		verbose      bool
		debug        bool
		socket       string
		timeout      string
		noColor      bool
		showVersion  bool
	)

	cmd := &cobra.Command{
		Use:           "iofog-agent",
		Short:         "Local CLI for the ioFog Agent daemon",
		Long:          rootLongHelp(),
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			format, err := output.ParseFormat(outputFormat)
			if err != nil {
				return run.NewCLIError(run.CodeInvalidArgument, err.Error(), err)
			}
			opts := ui.Options{Quiet: quiet, NoColor: noColor}
			appCtx = run.NewCLIContext(opts, format)
			appCtx.Out = cmd.OutOrStdout()
			appCtx.ErrOut = cmd.ErrOrStderr()
			appCtx.UI = ui.NewWithWriters(appCtx.Out, appCtx.ErrOut, opts)
			appCtx.Verbose = verbose
			appCtx.Debug = debug
			appCtx.Socket = socket
			appCtx.Timeout = timeout
			appCtx.Version = Version
			appCtx.BuildTime = BuildTime
			appCtx.GitCommit = GitCommit
			appCtx.Client = newClient()
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if showVersion {
				return runVersion(appCtx)
			}
			return cmd.Help()
		},
	}

	cmd.PersistentFlags().StringVarP(&outputFormat, "output", "o", "human", "Output format: human, json, yaml")
	cmd.PersistentFlags().BoolVar(&quiet, "quiet", false, "Suppress interactive progress output")
	cmd.PersistentFlags().BoolVar(&verbose, "verbose", false, "Verbose logging")
	cmd.PersistentFlags().BoolVar(&debug, "debug", false, "Debug logging")
	cmd.PersistentFlags().StringVar(&socket, "socket", "", "LocalAPI unix socket path")
	cmd.PersistentFlags().StringVar(&timeout, "timeout", "", "Request timeout")
	cmd.PersistentFlags().BoolVar(&noColor, "no-color", false, "Disable color and interactive UX")
	cmd.Flags().BoolVar(&showVersion, "version", false, "Print CLI and daemon version")
	registerOutputFlagCompletion(cmd)
	cmd.SetHelpTemplate(cliHelpTemplate)
	cmd.SetHelpFunc(printCommandHelp)

	cmd.AddCommand(newDeployCommand())
	cmd.AddCommand(newSystemCommand())
	cmd.AddCommand(newMSCommand())
	cmd.AddCommand(newRegistryCommand())
	cmd.AddCommand(newRuntimeClassCommand())
	cmd.AddCommand(newImageCommand())
	cmd.AddCommand(newAuthCommand())
	cmd.AddCommand(newProvisionCommand())
	cmd.AddCommand(newDeprovisionCommand())
	cmd.AddCommand(newConfigCommand())
	cmd.AddCommand(newDeprecatedTopLevelCommand("cert", "use `iofog-agent config cert` instead of top-level cert"))
	cmd.AddCommand(newDeprecatedTopLevelCommand("switch", "use `iofog-agent config switch` instead of top-level switch"))
	cmd.AddCommand(newDeprecatedTopLevelCommand("start", "top-level start is removed; start the daemon with iofog-agentd or systemctl start iofog-agentd"))
	cmd.AddCommand(newDeprecatedTopLevelCommand("stop", "use `iofog-agent system stop` instead of top-level stop"))
	cmd.AddCommand(newDeprecatedTopLevelCommand("prune", "use `iofog-agent system prune` instead of top-level prune"))

	cmd.AddCommand(newCompletionCommand(cmd))
	cmd.AddCommand(newDocumentationCommand(cmd))

	return cmd
}

func rootLongHelp() string {
	return strings.TrimSpace(`Local CLI for the ioFog Agent daemon.

Use "iofog-agent <command> --help" for command-specific usage.`)
}
