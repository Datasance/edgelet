package registry

import (
	"fmt"
	"strings"

	"github.com/eclipse-iofog/agent/internal/cli/output"
	"github.com/eclipse-iofog/agent/internal/cli/run"
)

// InspectResult carries registry inspect outcome.
type InspectResult struct {
	Human string
	Data  map[string]interface{}
}

// InspectArgs holds parsed registry inspect arguments.
type InspectArgs struct {
	ID            string
	PasswordPlain bool
}

// ParseInspectArgs parses registry inspect CLI arguments.
func ParseInspectArgs(args []string) (*InspectArgs, error) {
	if len(args) == 0 {
		return nil, run.NewCLIError(run.CodeInvalidArgument, "usage: iofog-agent registry inspect <id> [--password-plain]", nil)
	}
	id := strings.TrimSpace(args[0])
	if id == "" {
		return nil, run.NewCLIError(run.CodeInvalidArgument, "usage: iofog-agent registry inspect <id> [--password-plain]", nil)
	}
	parsed := &InspectArgs{ID: id}
	for i := 1; i < len(args); i++ {
		switch strings.TrimSpace(args[i]) {
		case "--password-plain":
			parsed.PasswordPlain = true
		default:
			return nil, run.NewCLIError(run.CodeInvalidArgument, fmt.Sprintf("unknown flag %s", args[i]), nil)
		}
	}
	return parsed, nil
}

// Inspect fetches a registry record and formats inspect output.
func Inspect(client run.V3Client, id string, passwordPlain bool) (*InspectResult, error) {
	if client == nil {
		return nil, run.NewCLIError(run.CodeInternal, "localapi client is nil", nil)
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, run.NewCLIError(run.CodeInvalidArgument, "registry id is required", nil)
	}
	item, err := client.RequestV3("GET", "/v3/deploy/registries/"+id, nil)
	if err != nil {
		return nil, run.MapAPIError(err)
	}
	human := output.FormatRegistryInspect(item, passwordPlain)
	if strings.HasPrefix(human, "Error[") {
		return nil, run.NewCLIError(run.CodeNotFound, "registry not found", nil)
	}
	return &InspectResult{Human: human, Data: item}, nil
}
