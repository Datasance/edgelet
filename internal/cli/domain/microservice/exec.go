package microservice

import (
	"strings"

	"github.com/eclipse-iofog/edgelet/internal/cli/client"
	"github.com/eclipse-iofog/edgelet/internal/cli/output"
	"github.com/eclipse-iofog/edgelet/internal/cli/run"
)

// Exec runs a command inside a microservice via EdgeletAPI exec session.
func Exec(c *client.Client, id string, command []string) error {
	if c == nil {
		return run.NewCLIError(run.CodeInternal, "edgeletapi client is nil", nil)
	}
	payload := map[string]any{
		"command": command,
		"tty":     true,
		"stdin":   true,
		"stdout":  true,
		"stderr":  true,
	}
	result, err := c.Request("POST", "/v1/ms/"+id+"/exec/sessions", payload)
	if err != nil {
		return run.MapAPIError(err)
	}
	sessionID := output.MapValueAsString(result, "sessionId")
	if sessionID == "" || sessionID == "<unknown>" {
		return run.NewCLIError(run.CodeInternal, "exec session id missing from response", nil)
	}
	exitCode, attachErr := client.AttachExecSession(c, id, sessionID)
	if attachErr != nil {
		return run.NewCLIError(run.CodeInternal, attachErr.Error(), attachErr)
	}
	if exitCode != 0 {
		return run.NewExecExitError(exitCode)
	}
	return nil
}

// ParseExecCommand extracts the remote command from ms exec args after the id.
// When Cobra parses "exec <id> -- cmd...", it strips "--" before RunE, so any
// remaining positional tokens are treated as the command.
func ParseExecCommand(args []string) ([]string, error) {
	for i, arg := range args {
		if arg == "--" {
			return args[i+1:], nil
		}
	}
	return args, nil
}

// ParseExecArgs validates ms exec positional args.
func ParseExecArgs(args []string) (id string, command []string, err error) {
	if len(args) < 1 {
		return "", nil, run.NewCLIError(run.CodeInvalidArgument, "usage: edgelet ms exec <id> [-- <command...>]", nil)
	}
	id = strings.TrimSpace(args[0])
	command, err = ParseExecCommand(args[1:])
	return id, command, err
}
