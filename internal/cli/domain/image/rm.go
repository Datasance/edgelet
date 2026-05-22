package image

import (
	"strings"

	"github.com/eclipse-iofog/agent/internal/cli/output"
	"github.com/eclipse-iofog/agent/internal/cli/run"
)

// RemoveResult carries image remove outcome.
type RemoveResult struct {
	Human string
	Data  map[string]interface{}
}

// Remove deletes an image by selector.
func Remove(client run.V3Client, selector string) (*RemoveResult, error) {
	if client == nil {
		return nil, run.NewCLIError(run.CodeInternal, "localapi client is nil", nil)
	}
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return nil, run.NewCLIError(run.CodeInvalidArgument, "selector is required", nil)
	}
	data, err := client.RequestV3("POST", "/v3/images:remove", map[string]interface{}{"selector": selector})
	if err != nil {
		return nil, run.MapAPIError(err)
	}
	return &RemoveResult{
		Human: output.FormatV3Human("/v3/images:remove", data),
		Data:  data,
	}, nil
}
