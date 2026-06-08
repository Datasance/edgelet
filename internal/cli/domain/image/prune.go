package image

import (
	"github.com/eclipse-iofog/edgelet/internal/cli/domain/prune"
	"github.com/eclipse-iofog/edgelet/internal/cli/output"
	"github.com/eclipse-iofog/edgelet/internal/cli/run"
)

// PruneResult carries image prune outcome.
type PruneResult struct {
	Human string
	Data  map[string]any
}

const imagePruneUsage = "usage: edgelet image prune [dangling]"

// Prune removes dangling images.
func Prune(client run.EdgeletAPIClient, args []string) (*PruneResult, error) {
	if client == nil {
		return nil, run.NewCLIError(run.CodeInternal, "edgeletapi client is nil", nil)
	}
	mode, err := prune.ParseImageMode(args, imagePruneUsage)
	if err != nil {
		return nil, err
	}
	path := "/v1/images:prune"
	if mode != "" {
		path += "?mode=" + mode
	}
	data, reqErr := client.Request("POST", path, nil)
	if reqErr != nil {
		return nil, run.MapAPIError(reqErr)
	}
	return &PruneResult{
		Human: output.FormatEdgeletAPIHuman(path, data),
		Data:  data,
	}, nil
}
