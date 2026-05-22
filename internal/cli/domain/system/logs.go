package system

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/eclipse-iofog/agent/internal/cli/client"
	"github.com/eclipse-iofog/agent/internal/cli/output"
	"github.com/eclipse-iofog/agent/internal/cli/run"
)

const logsHumanOnlyMsg = "logs output supports human format only"

// LogsOptions carries daemon log query flags.
type LogsOptions struct {
	Follow     bool
	Tail       string
	Since      string
	Until      string
	Timestamps bool
}

// ParseLogsOptions parses system logs CLI flags.
func ParseLogsOptions(args []string) (*LogsOptions, error) {
	opts := &LogsOptions{Tail: "100"}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--follow", "-f":
			opts.Follow = true
		case "--tail":
			if i+1 >= len(args) {
				return nil, run.NewCLIError(run.CodeInvalidArgument, "--tail requires a number", nil)
			}
			opts.Tail = strings.TrimSpace(args[i+1])
			i++
		case "--since":
			if i+1 >= len(args) {
				return nil, run.NewCLIError(run.CodeInvalidArgument, "--since requires an ISO8601 timestamp", nil)
			}
			opts.Since = strings.TrimSpace(args[i+1])
			i++
		case "--until":
			if i+1 >= len(args) {
				return nil, run.NewCLIError(run.CodeInvalidArgument, "--until requires an ISO8601 timestamp", nil)
			}
			opts.Until = strings.TrimSpace(args[i+1])
			i++
		case "--timestamps":
			opts.Timestamps = true
		case "--help", "-h", "-?":
			return nil, run.NewCLIError(run.CodeInvalidArgument, "usage: iofog-agent system logs [--follow] [--tail N] [--since ISO8601] [--until ISO8601] [--timestamps]", nil)
		default:
			return nil, run.NewCLIError(run.CodeInvalidArgument, fmt.Sprintf("unknown flag %s", args[i]), nil)
		}
	}
	return opts, nil
}

// FetchLogs retrieves buffered daemon logs.
func FetchLogs(ctx *run.CLIContext, api client.V3API, opts LogsOptions) error {
	if ctx != nil && ctx.Format.IsStructured() {
		return run.NewCLIError(run.CodeInvalidArgument, logsHumanOnlyMsg, nil)
	}
	path := "/v3/system/logs?tailLines=" + url.QueryEscape(opts.Tail)
	if opts.Since != "" {
		path += "&since=" + url.QueryEscape(opts.Since)
	}
	if opts.Until != "" {
		path += "&until=" + url.QueryEscape(opts.Until)
	}
	result, err := api.RequestV3("GET", path, nil)
	if err != nil {
		return run.MapAPIError(err)
	}
	text := output.FormatLogEntries(result, opts.Timestamps)
	return run.WriteHuman(ctx, strings.TrimRight(text, "\n"))
}

// StreamLogs follows daemon logs over WebSocket.
func StreamLogs(ctx *run.CLIContext, c *client.Client, opts LogsOptions) error {
	if ctx != nil && ctx.Format.IsStructured() {
		return run.NewCLIError(run.CodeInvalidArgument, logsHumanOnlyMsg, nil)
	}
	if c == nil {
		return run.NewCLIError(run.CodeInternal, "localapi client is nil", nil)
	}
	if err := client.StreamSystemLogs(c, opts.Tail, opts.Since, opts.Until, opts.Timestamps); err != nil {
		return run.NewCLIError(run.CodeInternal, err.Error(), err)
	}
	return nil
}
