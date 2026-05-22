package image

import (
	"github.com/eclipse-iofog/agent/internal/cli/domain/prune"
	"github.com/eclipse-iofog/agent/internal/cli/output"
	"github.com/eclipse-iofog/agent/internal/cli/run"
)

// PruneResult carries image prune outcome.
type PruneResult struct {
	Human string
	Data  map[string]interface{}
}

const imagePruneUsage = "usage: iofog-agent image prune [dangling]"

// Prune removes dangling images.
func Prune(client run.V3Client, args []string) (*PruneResult, error) {
	if client == nil {
		return nil, run.NewCLIError(run.CodeInternal, "localapi client is nil", nil)
	}
	mode, err := prune.ParseImageMode(args, imagePruneUsage)
	if err != nil {
		return nil, err
	}
	path := "/v3/images:prune"
	if mode != "" {
		path += "?mode=" + mode
	}
	data, reqErr := client.RequestV3("POST", path, nil)
	if reqErr != nil {
		return nil, run.MapAPIError(reqErr)
	}
	return &PruneResult{
		Human: output.FormatV3Human(path, data),
		Data:  data,
	}, nil
}
