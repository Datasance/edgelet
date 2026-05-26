package system

import (
	"net/url"
	"strings"

	"github.com/datasance/edgelet/internal/cli/client"
	"github.com/datasance/edgelet/internal/cli/domain/logs"
	"github.com/datasance/edgelet/internal/cli/output"
	"github.com/datasance/edgelet/internal/cli/run"
)

const logsHumanOnlyMsg = "logs output supports human format only"

// FetchLogs retrieves buffered daemon logs.
func FetchLogs(ctx *run.CLIContext, api client.EdgeletAPI, opts logs.Options) error {
	if ctx != nil && ctx.Format.IsStructured() {
		return run.NewCLIError(run.CodeInvalidArgument, logsHumanOnlyMsg, nil)
	}
	path := "/v1/system/logs?tailLines=" + url.QueryEscape(opts.Tail)
	if opts.Since != "" {
		path += "&since=" + url.QueryEscape(opts.Since)
	}
	if opts.Until != "" {
		path += "&until=" + url.QueryEscape(opts.Until)
	}
	result, err := api.Request("GET", path, nil)
	if err != nil {
		return run.MapAPIError(err)
	}
	text := output.FormatLogEntries(result, opts.Timestamps)
	return run.WriteHuman(ctx, strings.TrimRight(text, "\n"))
}

// StreamLogs follows daemon logs over WebSocket.
func StreamLogs(ctx *run.CLIContext, c *client.Client, opts logs.Options) error {
	if ctx != nil && ctx.Format.IsStructured() {
		return run.NewCLIError(run.CodeInvalidArgument, logsHumanOnlyMsg, nil)
	}
	if c == nil {
		return run.NewCLIError(run.CodeInternal, "edgeletapi client is nil", nil)
	}
	if err := client.StreamSystemLogs(c, opts.Tail, opts.Since, opts.Until, opts.Timestamps); err != nil {
		return run.NewCLIError(run.CodeInternal, err.Error(), err)
	}
	return nil
}
