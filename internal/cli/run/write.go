package run

import (
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/datasance/edgelet/internal/cli/client"
	"github.com/datasance/edgelet/internal/cli/output"
)

const daemonUnavailableMessage = "Edgelet daemon is not running. Start it with `edgelet` or `systemctl start edgelet`."

// RequireDaemon returns DAEMON_UNAVAILABLE when the daemon is unreachable.
func RequireDaemon(client V3Client) error {
	if client == nil || client.IsDaemonRunning() {
		return nil
	}
	return NewCLIError(CodeDaemonUnavailable, daemonUnavailableMessage, nil)
}

// MapAPIError converts LocalAPI errors to CLIError values.
func MapAPIError(err error) error {
	if err == nil {
		return nil
	}
	var apiErr *client.V3APIError
	if errors.As(err, &apiErr) {
		code := apiErr.Code
		if code == "" {
			code = "HTTP_ERROR"
		}
		return NewCLIError(code, apiErr.Message, err)
	}
	return NewCLIError(CodeInternal, err.Error(), err)
}

// WriteRouteData writes API payload to stdout using the selected output format.
func WriteRouteData(ctx *CLIContext, routePath string, data map[string]interface{}) error {
	if ctx == nil {
		return NewCLIError(CodeInternal, "cli context is nil", nil)
	}
	if len(data) == 0 {
		return nil
	}
	if ctx.Format.IsStructured() {
		formatter, err := output.NewFormatter(ctx.Format)
		if err != nil {
			return NewCLIError(CodeInvalidArgument, err.Error(), err)
		}
		raw, err := formatter.Format(data)
		if err != nil {
			return MapAPIError(err)
		}
		return writeBytes(ctx.Out, raw)
	}

	human := output.FormatV3Human(routePath, data)
	if human == "" {
		raw, err := json.MarshalIndent(data, "", "  ")
		if err != nil {
			return MapAPIError(err)
		}
		return writeBytes(ctx.Out, raw)
	}
	return writeBytes(ctx.Out, []byte(human))
}

// WriteValue writes an arbitrary payload to stdout.
func WriteValue(ctx *CLIContext, value any) error {
	if ctx == nil {
		return NewCLIError(CodeInternal, "cli context is nil", nil)
	}
	if value == nil {
		return nil
	}
	if ctx.Format.IsStructured() {
		formatter, err := output.NewFormatter(ctx.Format)
		if err != nil {
			return NewCLIError(CodeInvalidArgument, err.Error(), err)
		}
		raw, err := formatter.Format(value)
		if err != nil {
			return MapAPIError(err)
		}
		return writeBytes(ctx.Out, raw)
	}

	switch typed := value.(type) {
	case string:
		return writeBytes(ctx.Out, []byte(strings.TrimRight(typed, "\n")))
	default:
		formatter, err := output.NewFormatter(output.FormatHuman)
		if err != nil {
			return MapAPIError(err)
		}
		raw, err := formatter.Format(value)
		if err != nil {
			return MapAPIError(err)
		}
		return writeBytes(ctx.Out, raw)
	}
}

// WriteHumanSuccess writes a human success message to stderr.
func WriteHumanSuccess(ctx *CLIContext, message string) error {
	if ctx == nil {
		return NewCLIError(CodeInternal, "cli context is nil", nil)
	}
	if ctx.UI == nil {
		return NewCLIError(CodeInternal, "cli ui is nil", nil)
	}
	if strings.TrimSpace(message) == "" {
		return nil
	}
	ctx.UI.WriteSuccess(message)
	return nil
}

// WriteHumanMutationResult writes hybrid mutation output for human mode.
// The first line is a success summary on stderr; any remaining lines go to stdout.
func WriteHumanMutationResult(ctx *CLIContext, human string) error {
	if ctx == nil {
		return NewCLIError(CodeInternal, "cli context is nil", nil)
	}
	if ctx.UI == nil {
		return NewCLIError(CodeInternal, "cli ui is nil", nil)
	}
	human = strings.TrimRight(human, "\n")
	if human == "" {
		return nil
	}
	summary, body := splitSummaryBody(human)
	ctx.UI.WriteSuccess(summary)
	if body != "" {
		return WriteHuman(ctx, body)
	}
	return nil
}

// WriteHumanConfigResult writes human config mutation output to stderr.
// The summary line gets a colored marker; accepted/rejected detail stays plain.
// Returns exit code 2 when hasRejections is true.
func WriteHumanConfigResult(ctx *CLIContext, human string, hasRejections bool) error {
	if ctx == nil {
		return NewCLIError(CodeInternal, "cli context is nil", nil)
	}
	if ctx.UI == nil {
		return NewCLIError(CodeInternal, "cli ui is nil", nil)
	}
	human = strings.TrimRight(human, "\n")
	if human == "" {
		return nil
	}
	summary, body := splitSummaryBody(human)
	if hasRejections {
		ctx.UI.WriteError(summary)
	} else {
		ctx.UI.WriteSuccess(summary)
	}
	if body != "" {
		ctx.UI.WritePlain(body)
	}
	if hasRejections {
		return NewDisplayedCLIError(CodeInvalidArgument, summary)
	}
	return nil
}

func splitSummaryBody(human string) (summary, body string) {
	idx := strings.Index(human, "\n")
	if idx < 0 {
		return human, ""
	}
	return human[:idx], human[idx+1:]
}

// WriteHuman writes a preformatted human string to stdout.
func WriteHuman(ctx *CLIContext, text string) error {
	if ctx == nil {
		return NewCLIError(CodeInternal, "cli context is nil", nil)
	}
	if strings.TrimSpace(text) == "" {
		return nil
	}
	return writeBytes(ctx.Out, []byte(strings.TrimRight(text, "\n")))
}

func writeBytes(w io.Writer, raw []byte) error {
	if len(raw) == 0 {
		return nil
	}
	if _, err := io.WriteString(w, string(raw)); err != nil {
		return err
	}
	if !strings.HasSuffix(string(raw), "\n") {
		_, err := io.WriteString(w, "\n")
		return err
	}
	return nil
}
