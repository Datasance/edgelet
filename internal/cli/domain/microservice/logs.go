package microservice

import (
	"strings"

	"github.com/datasance/edgelet/internal/cli/client"
	"github.com/datasance/edgelet/internal/cli/domain/logs"
	"github.com/datasance/edgelet/internal/cli/output"
	"github.com/datasance/edgelet/internal/cli/run"
)

const logsHumanOnlyMsg = "logs output supports human format only"

// FetchLogs retrieves buffered microservice logs.
func FetchLogs(ctx *run.CLIContext, api client.EdgeletAPI, id string, opts logs.Options) error {
	if ctx != nil && ctx.Format.IsStructured() {
		return run.NewCLIError(run.CodeInvalidArgument, logsHumanOnlyMsg, nil)
	}
	path := "/v1/ms/" + id + "/logs?tail=" + opts.Tail
	if opts.Since != "" {
		path += "&since=" + opts.Since
	}
	if opts.Until != "" {
		path += "&until=" + opts.Until
	}
	result, err := api.Request("GET", path, nil)
	if err != nil {
		return run.MapAPIError(err)
	}
	text := formatMSLogEntries(result, opts.Timestamps)
	return run.WriteHuman(ctx, strings.TrimRight(text, "\n"))
}

// StreamLogs follows microservice logs over WebSocket.
func StreamLogs(ctx *run.CLIContext, c *client.Client, id string, opts logs.Options) error {
	if ctx != nil && ctx.Format.IsStructured() {
		return run.NewCLIError(run.CodeInvalidArgument, logsHumanOnlyMsg, nil)
	}
	if c == nil {
		return run.NewCLIError(run.CodeInternal, "edgeletapi client is nil", nil)
	}
	if err := client.StreamMSLogs(c, id, opts.Tail, opts.Since, opts.Until, opts.Timestamps); err != nil {
		return run.NewCLIError(run.CodeInternal, err.Error(), err)
	}
	return nil
}

func formatMSLogEntries(result map[string]interface{}, timestamps bool) string {
	rawEntries, _ := result["entries"].([]interface{})
	var b strings.Builder
	for _, raw := range rawEntries {
		entry, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		line := output.MapValueAsRawString(entry, "line")
		if timestamps {
			ts := output.MapValueAsString(entry, "ts")
			b.WriteString(ts + " ")
		}
		b.WriteString(line)
		if !strings.HasSuffix(line, "\n") {
			b.WriteString("\n")
		}
	}
	return b.String()
}
