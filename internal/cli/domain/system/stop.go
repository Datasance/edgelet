package system

import (
	"github.com/datasance/edgelet/internal/cli/output"
	"github.com/datasance/edgelet/internal/cli/run"
)

// StopResult carries daemon stop outcome.
type StopResult struct {
	Human string
	Data  map[string]interface{}
}

// Stop requests daemon shutdown via EdgeletAPI.
func Stop(client run.EdgeletAPIClient) (*StopResult, error) {
	if client == nil {
		return nil, run.NewCLIError(run.CodeInternal, "edgeletapi client is nil", nil)
	}
	data, err := client.Request("POST", "/v1/system/stop", nil)
	if err != nil {
		return nil, run.MapAPIError(err)
	}
	human := output.FormatEdgeletAPIHuman("/v1/system/stop", data)
	return &StopResult{Human: human, Data: data}, nil
}
