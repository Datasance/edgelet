package provision

import (
	"strings"

	"github.com/datasance/edgelet/internal/cli/output"
	"github.com/datasance/edgelet/internal/cli/run"
)

// Result carries provision outcome.
type Result struct {
	Human string
	Data  map[string]interface{}
}

// Provision registers the agent with a controller using a provisioning key.
func Provision(client run.V3Client, key string) (*Result, error) {
	if client == nil {
		return nil, run.NewCLIError(run.CodeInternal, "localapi client is nil", nil)
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, run.NewCLIError(run.CodeInvalidArgument, "provision command requires a provisioning key", nil)
	}
	result, err := client.RequestV3("POST", "/v1/system/provision", map[string]string{"provisioningKey": key})
	if err != nil {
		return nil, run.MapAPIError(err)
	}
	agentUUID := output.MapValueAsString(result, "agentUuid")
	if agentUUID == "<unknown>" {
		agentUUID = output.MapValueAsString(result, "iofogUuid")
	}
	if agentUUID == "<unknown>" {
		infoResult, infoErr := client.RequestV3("GET", "/v1/system/info", nil)
		if infoErr == nil {
			agentUUID = output.MapValueAsString(infoResult, "iofogUuid")
		}
	}
	return &Result{
		Human: output.FormatProvisionSuccess(agentUUID),
		Data:  result,
	}, nil
}
