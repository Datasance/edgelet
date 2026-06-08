package config

import (
	"strings"

	"github.com/eclipse-iofog/edgelet/internal/cli/output"
	"github.com/eclipse-iofog/edgelet/internal/cli/run"
)

// SwitchResult carries profile switch outcome.
type SwitchResult struct {
	Human string
	Data  map[string]any
}

// SwitchProfile validates and switches the active configuration profile.
func SwitchProfile(client run.EdgeletAPIClient, profile string) (*SwitchResult, error) {
	if client == nil {
		return nil, run.NewCLIError(run.CodeInternal, "edgeletapi client is nil", nil)
	}
	profile = strings.TrimSpace(profile)
	if profile == "" {
		return nil, run.NewCLIError(run.CodeInvalidArgument, "switch command requires a profile", nil)
	}
	switch profile {
	case "dev", "development", "prod", "production", "def", "default":
	default:
		return nil, run.NewCLIError(run.CodeInvalidArgument, "profile must be one of dev|prod|def", nil)
	}
	data, err := client.Request("POST", "/v1/system/config/switch", map[string]any{
		"profile": profile,
	})
	if err != nil {
		return nil, run.MapAPIError(err)
	}
	return &SwitchResult{
		Human: output.FormatEdgeletAPIHuman("/v1/system/config/switch", data),
		Data:  data,
	}, nil
}
