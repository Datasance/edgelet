package cmd

import (
	"github.com/datasance/edgelet/internal/cli/domain/config"
	"github.com/datasance/edgelet/internal/cli/run"
	"github.com/spf13/cobra"
)

var configFlags = config.NewFlagSet()

func newConfigCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "config",
		Short:   "Update agent configuration",
		Long:    config.CommandLong(),
		Example: config.CommandExamples(),
		Args:    cobra.NoArgs,
		RunE:    runConfigPatch,
	}

	configFlags.Register(cmd)

	cmd.AddCommand(
		&cobra.Command{
			Use:   "cert <base64-encoded-certificate>",
			Short: "Install controller CA certificate",
			Long:  config.CertCommandLong(),
			Args:  cobra.ExactArgs(1),
			RunE:  runConfigCert,
		},
		&cobra.Command{
			Use:   "switch <dev|prod|def>",
			Short: "Switch configuration profile",
			Args:  cobra.ExactArgs(1),
			RunE:  runConfigSwitch,
		},
	)

	return cmd
}

func runConfigPatch(cmd *cobra.Command, _ []string) error {
	if appCtx == nil {
		return run.NewCLIError(run.CodeInternal, "cli context is nil", nil)
	}
	if err := run.RequireDaemon(appCtx.Client); err != nil {
		return err
	}
	setMap, err := configFlags.Collect(cmd.Flags())
	if err != nil {
		return run.NewCLIError(run.CodeInvalidArgument, err.Error(), err)
	}
	if appCtx.Format.IsStructured() {
		result, err := config.ApplySetMap(appCtx.Client, setMap)
		if err != nil {
			return err
		}
		return run.WriteValue(appCtx, result.Data)
	}

	spin := appCtx.UI.StartSpinner("Updating configuration...")
	result, err := config.ApplySetMap(appCtx.Client, setMap)
	spin.Stop()
	if err != nil {
		return err
	}
	if pending, _ := result.Data["pendingRestart"].(bool); pending {
		if msg, _ := result.Data["message"].(string); msg != "" {
			result.Human += "\n" + msg
		} else {
			result.Human += "\nRestart required: systemctl restart edgelet"
		}
	}
	return run.WriteHumanConfigResult(appCtx, result.Human, config.HasRejections(result.Data))
}

func runConfigCert(cmd *cobra.Command, args []string) error {
	if appCtx == nil {
		return run.NewCLIError(run.CodeInternal, "cli context is nil", nil)
	}
	if err := run.RequireDaemon(appCtx.Client); err != nil {
		return err
	}
	if appCtx.Format.IsStructured() {
		result, err := config.ApplyCert(appCtx.Client, args[0])
		if err != nil {
			return err
		}
		return run.WriteValue(appCtx, result.Data)
	}

	spin := appCtx.UI.StartSpinner("Installing controller certificate...")
	result, err := config.ApplyCert(appCtx.Client, args[0])
	spin.Stop()
	if err != nil {
		return err
	}
	return run.WriteHumanSuccess(appCtx, result.Human)
}

func runConfigSwitch(cmd *cobra.Command, args []string) error {
	if appCtx == nil {
		return run.NewCLIError(run.CodeInternal, "cli context is nil", nil)
	}
	if err := run.RequireDaemon(appCtx.Client); err != nil {
		return err
	}
	if appCtx.Format.IsStructured() {
		result, err := config.SwitchProfile(appCtx.Client, args[0])
		if err != nil {
			return err
		}
		return run.WriteValue(appCtx, result.Data)
	}

	spin := appCtx.UI.StartSpinner("Switching configuration profile...")
	result, err := config.SwitchProfile(appCtx.Client, args[0])
	spin.Stop()
	if err != nil {
		return err
	}
	return run.WriteHumanSuccess(appCtx, result.Human)
}

func newDeprecatedTopLevelCommand(name, message string) *cobra.Command {
	return &cobra.Command{
		Use:    name,
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return run.NewCLIError(run.CodeInvalidArgument, message, nil)
		},
	}
}
