package provision

import (
	"strings"

	"github.com/datasance/edgelet/internal/cli/run"
)

// DeprovisionResult carries deprovision outcome.
type DeprovisionResult struct {
	Human string
	Data  map[string]interface{}
}

// DeprovisionRequest carries deprovision options.
type DeprovisionRequest struct {
	Scope string
}

// Deprovision removes agent provisioning and optionally preserves local microservices.
func Deprovision(client run.EdgeletAPIClient, req DeprovisionRequest) (*DeprovisionResult, error) {
	if client == nil {
		return nil, run.NewCLIError(run.CodeInternal, "edgeletapi client is nil", nil)
	}
	scope := strings.ToLower(strings.TrimSpace(req.Scope))
	if scope == "" {
		scope = "all"
	}
	if scope != "all" && scope != "local" {
		return nil, run.NewCLIError(run.CodeInvalidArgument, "--scope requires all|local", nil)
	}
	path := "/v1/system/provision"
	if scope != "all" {
		path += "?scope=" + scope
	}
	data, err := client.Request("DELETE", path, nil)
	if err != nil {
		return nil, run.MapAPIError(err)
	}
	human := "agent deprovisioned successfully; started cleanup of managed and local microservices"
	if scope == "local" {
		human = "agent deprovisioned successfully; preserving local microservices"
	}
	return &DeprovisionResult{Human: human, Data: data}, nil
}
