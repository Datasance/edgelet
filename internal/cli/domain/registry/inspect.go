package registry

import (
	"strings"

	"github.com/datasance/edgelet/internal/cli/output"
	"github.com/datasance/edgelet/internal/cli/run"
)

// InspectResult carries registry inspect outcome.
type InspectResult struct {
	Human string
	Data  map[string]interface{}
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
	item, err := client.RequestV3("GET", "/v1/deploy/registries/"+id, nil)
	if err != nil {
		return nil, run.MapAPIError(err)
	}
	human := output.FormatRegistryInspect(item, passwordPlain)
	if strings.HasPrefix(human, "Error[") {
		return nil, run.NewCLIError(run.CodeNotFound, "registry not found", nil)
	}
	return &InspectResult{Human: human, Data: item}, nil
}
