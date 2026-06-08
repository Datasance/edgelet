package config

import (
	"github.com/datasance/edgelet/internal/cli/output"
	"github.com/datasance/edgelet/internal/cli/run"
)

// Result carries config patch outcome.
type Result struct {
	Human string
	Data  map[string]any
}

// Patch GETs current config, PATCHes keys, and formats the mutation output.
func Patch(client run.EdgeletAPIClient, setMap map[string]any) (human string, data map[string]any, err error) {
	if client == nil {
		return "", nil, run.NewCLIError(run.CodeInternal, "edgeletapi client is nil", nil)
	}
	payload := map[string]any{"set": setMap}
	before, _ := client.Request("GET", "/v1/system/config", nil)
	after, reqErr := client.Request("PATCH", "/v1/system/config", payload)
	if reqErr != nil {
		return "", nil, run.MapAPIError(reqErr)
	}
	return output.FormatConfigMutationOutput(setMap, before, after), after, nil
}

// HasRejections reports whether a config PATCH response includes rejected keys.
func HasRejections(data map[string]any) bool {
	if len(data) == 0 {
		return false
	}
	errorMap, ok := data["errorMap"].(map[string]any)
	if !ok {
		errorMap = map[string]any{}
	}
	return len(errorMap) > 0
}

// ApplySetMap validates and PATCHes the provided config keys.
func ApplySetMap(client run.EdgeletAPIClient, setMap map[string]any) (*Result, error) {
	if len(setMap) == 0 {
		return nil, run.NewCLIError(run.CodeInvalidArgument, "at least one config flag is required (see edgelet config --help)", nil)
	}
	human, data, err := Patch(client, setMap)
	if err != nil {
		return nil, err
	}
	return &Result{Human: human, Data: data}, nil
}
