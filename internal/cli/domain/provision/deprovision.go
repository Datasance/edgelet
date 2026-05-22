package provision

import (
	"fmt"
	"strings"

	"github.com/eclipse-iofog/agent/internal/cli/run"
)

// DeprovisionResult carries deprovision outcome.
type DeprovisionResult struct {
	Human string
	Data  map[string]interface{}
}

// Deprovision removes agent provisioning and optionally preserves local microservices.
func Deprovision(client run.V3Client, args []string) (*DeprovisionResult, error) {
	if client == nil {
		return nil, run.NewCLIError(run.CodeInternal, "localapi client is nil", nil)
	}
	scope := "all"
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--scope":
			if i+1 >= len(args) {
				return nil, run.NewCLIError(run.CodeInvalidArgument, "--scope requires all|local", nil)
			}
			scope = strings.ToLower(strings.TrimSpace(args[i+1]))
			i++
		case "--keep-local":
			scope = "local"
		case "-h", "--help", "-?":
			return nil, run.NewCLIError(run.CodeInvalidArgument, "usage: iofog-agent deprovision [--scope all|local] [--keep-local]", nil)
		default:
			return nil, run.NewCLIError(run.CodeInvalidArgument, fmt.Sprintf("unknown flag %s", args[i]), nil)
		}
	}
	if scope != "all" && scope != "local" {
		return nil, run.NewCLIError(run.CodeInvalidArgument, "--scope requires all|local", nil)
	}
	path := "/v3/system/provision"
	if scope != "all" {
		path += "?scope=" + scope
	}
	data, err := client.RequestV3("DELETE", path, nil)
	if err != nil {
		return nil, run.MapAPIError(err)
	}
	human := "agent deprovisioned successfully; started cleanup of managed and local microservices"
	if scope == "local" {
		human = "agent deprovisioned successfully; preserving local microservices"
	}
	return &DeprovisionResult{Human: human, Data: data}, nil
}
