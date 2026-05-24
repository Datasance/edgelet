package config

import (
	"strings"

	"github.com/datasance/edgelet/internal/cli/output"
	"github.com/datasance/edgelet/internal/cli/run"
)

// SwitchResult carries profile switch outcome.
type SwitchResult struct {
	Human string
	Data  map[string]interface{}
}

// SwitchProfile validates and switches the active configuration profile.
func SwitchProfile(client run.V3Client, profile string) (*SwitchResult, error) {
	if client == nil {
		return nil, run.NewCLIError(run.CodeInternal, "localapi client is nil", nil)
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
	data, err := client.RequestV3("POST", "/v1/system/config/switch", map[string]interface{}{
		"profile": profile,
	})
	if err != nil {
		return nil, run.MapAPIError(err)
	}
	return &SwitchResult{
		Human: output.FormatV3Human("/v1/system/config/switch", data),
		Data:  data,
	}, nil
}
