package cmd

import (
	"github.com/eclipse-iofog/agent/internal/cli/domain/config"
	"github.com/eclipse-iofog/agent/internal/cli/run"
	"github.com/spf13/cobra"
)

func newConfigCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Update agent configuration",
		Long:  "Set config keys as key/value pairs, or use config cert / config switch subcommands.",
		RunE:  runConfigPatch,
	}

	cmd.AddCommand(
		&cobra.Command{
			Use:   "cert <base64-encoded-certificate>",
			Short: "Install controller certificate",
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

func runConfigPatch(cmd *cobra.Command, args []string) error {
	if appCtx == nil {
		return run.NewCLIError(run.CodeInternal, "cli context is nil", nil)
	}
	if err := run.RequireDaemon(appCtx.Client); err != nil {
		return err
	}
	result, err := config.Apply(appCtx.Client, args)
	if err != nil {
		return err
	}
	return writeHumanOrData(appCtx, result.Human, result.Data)
}

func runConfigCert(cmd *cobra.Command, args []string) error {
	if appCtx == nil {
		return run.NewCLIError(run.CodeInternal, "cli context is nil", nil)
	}
	if err := run.RequireDaemon(appCtx.Client); err != nil {
		return err
	}
	result, err := config.ApplyCert(appCtx.Client, args[0])
	if err != nil {
		return err
	}
	return writeHumanOrRoute(appCtx, "/v3/system/controller/cert", result.Human, result.Data)
}

func runConfigSwitch(cmd *cobra.Command, args []string) error {
	if appCtx == nil {
		return run.NewCLIError(run.CodeInternal, "cli context is nil", nil)
	}
	if err := run.RequireDaemon(appCtx.Client); err != nil {
		return err
	}
	result, err := config.SwitchProfile(appCtx.Client, args[0])
	if err != nil {
		return err
	}
	return writeHumanOrRoute(appCtx, "/v3/system/config/switch", result.Human, result.Data)
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
