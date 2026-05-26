package image

import (
	"strings"

	"github.com/datasance/edgelet/internal/cli/output"
	"github.com/datasance/edgelet/internal/cli/run"
)

// RemoveResult carries image remove outcome.
type RemoveResult struct {
	Human string
	Data  map[string]interface{}
}

// Remove deletes an image by selector.
func Remove(client run.EdgeletAPIClient, selector string) (*RemoveResult, error) {
	if client == nil {
		return nil, run.NewCLIError(run.CodeInternal, "edgeletapi client is nil", nil)
	}
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return nil, run.NewCLIError(run.CodeInvalidArgument, "selector is required", nil)
	}
	data, err := client.Request("POST", "/v1/images:remove", map[string]interface{}{"selector": selector})
	if err != nil {
		return nil, run.MapAPIError(err)
	}
	return &RemoveResult{
		Human: output.FormatEdgeletAPIHuman("/v1/images:remove", data),
		Data:  data,
	}, nil
}
