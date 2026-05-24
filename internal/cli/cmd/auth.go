package cmd

import (
	"github.com/datasance/edgelet/internal/cli/domain/auth"
	"github.com/datasance/edgelet/internal/cli/run"
	"github.com/spf13/cobra"
)

func newAuthCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "auth",
		Short:   "Authentication operations",
		Long:    auth.CommandLong(),
		Example: auth.CommandExamples(),
	}

	cmd.AddCommand(
		&cobra.Command{
			Use:   "whoami",
			Short: "Show current auth identity",
			RunE:  runGET("/v1/auth/whoami"),
		},
		&cobra.Command{
			Use:   "tokens",
			Short: "List auth tokens",
			RunE:  runGET("/v1/auth/tokens"),
		},
		&cobra.Command{
			Use:   "revoke <jti>",
			Short: "Revoke an auth token",
			Args:  cobra.ExactArgs(1),
			RunE:  runAuthRevoke,
		},
	)

	return cmd
}

func runAuthRevoke(cmd *cobra.Command, args []string) error {
	if appCtx == nil {
		return run.NewCLIError(run.CodeInternal, "cli context is nil", nil)
	}
	if err := run.RequireDaemon(appCtx.Client); err != nil {
		return err
	}
	data, err := appCtx.Client.RequestV3("POST", "/v1/auth/tokens/revoke", map[string]interface{}{"jti": args[0]})
	if err != nil {
		return run.MapAPIError(err)
	}
	return run.WriteRouteData(appCtx, "/v1/auth/tokens/revoke", data)
}
