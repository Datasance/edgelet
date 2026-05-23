package image

import (
	"strings"

	"github.com/eclipse-iofog/agent/internal/cli/output"
	"github.com/eclipse-iofog/agent/internal/cli/run"
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
func Load(client run.V3Client, req LoadRequest) (*LoadResult, error) {
	if client == nil {
		return nil, run.NewCLIError(run.CodeInternal, "localapi client is nil", nil)
	}
	path := strings.TrimSpace(req.Path)
	if path == "" {
		return nil, run.NewCLIError(run.CodeInvalidArgument, "path is required", nil)
	}
	data, err := client.RequestV3("POST", "/v3/images:load", map[string]interface{}{"path": path})
	if err != nil {
		return nil, run.MapAPIError(err)
	}
	return &LoadResult{
		Human: output.FormatV3Human("/v3/images:load", data),
		Data:  data,
	}, nil
}
