package image

import (
	"strings"

	"github.com/datasance/edgelet/internal/cli/output"
	"github.com/datasance/edgelet/internal/cli/run"
)

// LoadResult carries image load outcome.
type LoadResult struct {
	Human string
	Data  map[string]interface{}
}

// LoadRequest carries image load options.
type LoadRequest struct {
	Path string
}

// Load imports an image archive from a tar file path.
func Load(client run.EdgeletAPIClient, req LoadRequest) (*LoadResult, error) {
	if client == nil {
		return nil, run.NewCLIError(run.CodeInternal, "edgeletapi client is nil", nil)
	}
	path := strings.TrimSpace(req.Path)
	if path == "" {
		return nil, run.NewCLIError(run.CodeInvalidArgument, "path is required", nil)
	}
	data, err := client.Request("POST", "/v1/images:load", map[string]interface{}{"path": path})
	if err != nil {
		return nil, run.MapAPIError(err)
	}
	return &LoadResult{
		Human: output.FormatEdgeletAPIHuman("/v1/images:load", data),
		Data:  data,
	}, nil
}
