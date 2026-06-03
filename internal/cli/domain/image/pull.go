package image

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/datasance/edgelet/internal/cli/client"
	"github.com/datasance/edgelet/internal/cli/output"
	"github.com/datasance/edgelet/internal/cli/run"
	"github.com/datasance/edgelet/internal/cli/ui"
)

// PullRequest carries image pull options.
type PullRequest struct {
	Image      string
	RegistryID int
	Platform   string
}

// PullResult is the image pull command outcome.
type PullResult struct {
	Data  map[string]interface{}
	Human string
}

// Pull executes async image pull with shared polling progress.
func Pull(ctx context.Context, api run.EdgeletAPIClient, uiProgress *ui.UI, req PullRequest) (*PullResult, error) {
	if api == nil {
		return nil, run.NewCLIError(run.CodeInternal, "edgeletapi client is nil", nil)
	}
	imageRef := strings.TrimSpace(req.Image)
	if imageRef == "" {
		return nil, run.NewCLIError(run.CodeInvalidArgument, "image is required", nil)
	}

	payload := map[string]interface{}{
		"image": imageRef,
		"async": true,
	}
	if req.RegistryID > 0 {
		payload["registryId"] = req.RegistryID
	}
	if req.Platform != "" {
		payload["platform"] = req.Platform
	}

	startResult, err := api.Request("POST", "/v1/images:pull", payload)
	if err != nil {
		return nil, run.MapAPIError(err)
	}
	operationID := client.OperationIDFromStart(startResult)
	if operationID == "" || operationID == "<unknown>" {
		return nil, run.NewCLIError(run.CodeInternal, "missing image pull operationId in response", nil)
	}

	progress := client.PollProgress{
		UI:           uiProgress,
		PercentLabel: "pulling image",
	}
	if uiProgress != nil {
		spin := uiProgress.StartSpinner("Pulling image...")
		defer spin.Stop()
		progress.Spinner = spin
	}

	final, _, err := client.PollAsyncOperation(ctx, client.PollConfig{
		Interval: 500 * time.Millisecond,
		Timeout:  client.PollTimeoutFor("image-pull"),
	}, func() (map[string]interface{}, error) {
		return api.Request("GET", "/v1/images:pull/"+operationID, nil)
	}, progress)
	if err != nil {
		return nil, run.MapAPIError(err)
	}

	status := strings.ToLower(strings.TrimSpace(output.MapValueAsString(final, "status")))
	if status == "failed" {
		errMsg := strings.TrimSpace(output.MapValueAsString(final, "error"))
		if errMsg == "" || errMsg == "<unknown>" {
			errMsg = "image pull failed"
		}
		return nil, run.NewCLIError(run.CodeInternal, errMsg, nil)
	}

	return &PullResult{Data: final, Human: formatPullHuman(final)}, nil
}

func formatPullHuman(result map[string]interface{}) string {
	return fmt.Sprintf(
		"image pulled successfully: %s (engine=%s, platform=%s)",
		output.MapValueAsString(result, "resolvedImage"),
		output.ValueOrDefault(output.MapValueAsString(result, "engine"), "<unknown>"),
		output.ValueOrDefault(output.MapValueAsString(result, "platform"), "<none>"),
	)
}
