package image

import (
	"context"
	"strings"
	"time"

	"github.com/eclipse-iofog/edgelet/internal/cli/client"
	"github.com/eclipse-iofog/edgelet/internal/cli/output"
	"github.com/eclipse-iofog/edgelet/internal/cli/run"
	"github.com/eclipse-iofog/edgelet/internal/cli/ui"
)

// LoadResult carries image load outcome.
type LoadResult struct {
	Human string
	Data  map[string]any
}

// LoadRequest carries image load options.
type LoadRequest struct {
	Path string
}

// Load imports an image archive from a tar or tar.gz file path.
func Load(ctx context.Context, clientAPI run.EdgeletAPIClient, uiProgress *ui.UI, req LoadRequest) (*LoadResult, error) {
	if clientAPI == nil {
		return nil, run.NewCLIError(run.CodeInternal, "edgeletapi client is nil", nil)
	}
	path := strings.TrimSpace(req.Path)
	if path == "" {
		return nil, run.NewCLIError(run.CodeInvalidArgument, "path is required", nil)
	}

	startResult, err := clientAPI.Request("POST", "/v1/images:load", map[string]any{"path": path})
	if err != nil {
		return nil, run.MapAPIError(err)
	}
	operationID := client.OperationIDFromStart(startResult)
	if operationID == "" || operationID == "<unknown>" {
		return nil, run.NewCLIError(run.CodeInternal, "missing image load operationId in response", nil)
	}

	progress := client.PollProgress{UI: uiProgress}
	if uiProgress != nil {
		spin := uiProgress.StartSpinner("Loading image archive...")
		defer spin.Stop()
		progress.Spinner = spin
	}

	final, _, err := client.PollAsyncOperation(ctx, client.PollConfig{
		Interval: 500 * time.Millisecond,
		Timeout:  client.PollTimeoutFor("image-load"),
	}, func() (map[string]any, error) {
		return clientAPI.Request("GET", "/v1/images:load/"+operationID, nil)
	}, progress)
	if err != nil {
		return nil, run.MapAPIError(err)
	}

	status := strings.ToLower(strings.TrimSpace(output.MapValueAsString(final, "status")))
	if status == "failed" {
		errMsg := strings.TrimSpace(output.MapValueAsString(final, "error"))
		if errMsg == "" || errMsg == "<unknown>" {
			errMsg = "image load failed"
		}
		return nil, run.NewCLIError(run.CodeInternal, errMsg, nil)
	}

	return &LoadResult{
		Human: output.FormatEdgeletAPIHuman("/v1/images:load", final),
		Data:  final,
	}, nil
}
