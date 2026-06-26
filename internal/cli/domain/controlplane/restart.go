package controlplane

import (
	"github.com/eclipse-iofog/edgelet/internal/cli/output"
	"github.com/eclipse-iofog/edgelet/internal/cli/run"
)

// RestartResult carries a control plane restart operation outcome.
type RestartResult struct {
	Human string
	Data  map[string]any
	Path  string
}

// Restart bounces the singleton control plane controller container.
func Restart(client run.EdgeletAPIClient, pull bool) (*RestartResult, error) {
	if client == nil {
		return nil, run.NewCLIError(run.CodeInternal, "edgeletapi client is nil", nil)
	}
	path := "/v1/system/controlplane/restart"
	if pull {
		path += "?pull=true"
	}
	data, err := client.Request("POST", path, nil)
	if err != nil {
		return nil, run.MapAPIError(err)
	}
	return &RestartResult{
		Human: output.FormatEdgeletAPIHuman(path, data),
		Data:  data,
		Path:  path,
	}, nil
}
