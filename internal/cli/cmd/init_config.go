package cmd

import (
	"fmt"

	"github.com/datasance/edgelet/internal/cli/run"
	"github.com/datasance/edgelet/internal/config"
	"github.com/datasance/edgelet/internal/utils"
	"github.com/spf13/cobra"
)

func newInitConfigCommand() *cobra.Command {
	var configPath string

	cmd := &cobra.Command{
		Use:   "init-config",
		Short: "Write default config if missing",
		Long: `Write the Edgelet default configuration template when no config file exists.

Does not overwrite an existing config. Uses /usr/share/edgelet/edgelet-config.yaml.sample
when installed, otherwise the embedded release template.`,
		Example: `  sudo edgelet init-config
  edgelet init-config --config-path /etc/edgelet/config.yaml`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if appCtx == nil {
				return run.NewCLIError(run.CodeInternal, "cli context is nil", nil)
			}
			path := configPath
			if path == "" {
				path = utils.ConfigYAMLPath
			}
			created, err := config.InitConfig(path)
			if err != nil {
				return run.NewCLIError(run.CodeInternal, err.Error(), err)
			}
			if created {
				return run.WriteHumanSuccess(appCtx, fmt.Sprintf("Config installed at %s", path))
			}
			return run.WriteHumanSuccess(appCtx, fmt.Sprintf("Config already exists at %s (unchanged)", path))
		},
	}

	cmd.Flags().StringVar(&configPath, "config-path", "", "Config file path (default: /etc/edgelet/config.yaml)")
	return cmd
}
