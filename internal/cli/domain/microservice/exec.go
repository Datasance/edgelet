package microservice

import (
	"strings"

	"github.com/eclipse-iofog/agent/internal/cli/client"
	"github.com/eclipse-iofog/agent/internal/cli/output"
	"github.com/eclipse-iofog/agent/internal/cli/run"
)

// Exec runs a command inside a microservice via LocalAPI exec session.
func Exec(c *client.Client, id string, command []string) error {
	if c == nil {
		return run.NewCLIError(run.CodeInternal, "localapi client is nil", nil)
	}
	payload := map[string]interface{}{
		"command": command,
		"tty":     true,
		"stdin":   true,
		"stdout":  true,
		"stderr":  true,
	}
	result, err := c.RequestV3("POST", "/v3/ms/"+id+"/exec/sessions", payload)
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

// ParseExecCommand extracts the command after a "--" separator.
func ParseExecCommand(args []string) ([]string, error) {
	command := make([]string, 0)
	for i := 0; i < len(args); i++ {
		if args[i] == "--" {
			command = append(command, args[i+1:]...)
			break
		}
		return nil, run.NewCLIError(run.CodeInvalidArgument, "unknown flag "+args[i], nil)
	}
	return command, nil
}

// ParseExecArgs validates ms exec positional args.
func ParseExecArgs(args []string) (id string, command []string, err error) {
	if len(args) < 1 {
		return "", nil, run.NewCLIError(run.CodeInvalidArgument, "usage: iofog-agent ms exec <id> [-- <command...>]", nil)
	}
	id = strings.TrimSpace(args[0])
	command, err = ParseExecCommand(args[1:])
	return id, command, err
}
