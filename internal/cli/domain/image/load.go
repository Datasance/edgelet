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

// ParseLoadArgs parses image load CLI flags.
func ParseLoadArgs(args []string) (*LoadRequest, error) {
	if len(args) < 1 {
		return nil, run.NewCLIError(run.CodeInvalidArgument, "usage: iofog-agent image load -f <path-to-tar-file>", nil)
	}
	path := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-f", "--file":
			if i+1 >= len(args) {
				return nil, run.NewCLIError(run.CodeInvalidArgument, "-f requires path-to-tar-file", nil)
			}
			path = strings.TrimSpace(args[i+1])
			i++
		case "-h", "--help", "-?":
			return nil, run.NewCLIError(run.CodeInvalidArgument, "usage: iofog-agent image load -f <path-to-tar-file>", nil)
		default:
			return nil, run.NewCLIError(run.CodeInvalidArgument, "unknown flag "+args[i], nil)
		}
	}
	if path == "" {
		return nil, run.NewCLIError(run.CodeInvalidArgument, "path is required", nil)
	}
	return &LoadRequest{Path: path}, nil
}

// Load imports an image archive from a tar file path.
func Load(client run.V3Client, req LoadRequest) (*LoadResult, error) {
	if client == nil {
		return nil, run.NewCLIError(run.CodeInternal, "localapi client is nil", nil)
	}
	data, err := client.RequestV3("POST", "/v3/images:load", map[string]interface{}{"path": req.Path})
	if err != nil {
		return nil, run.MapAPIError(err)
	}
	return &LoadResult{
		Human: output.FormatV3Human("/v3/images:load", data),
		Data:  data,
	}, nil
}
