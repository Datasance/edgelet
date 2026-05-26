package config

import (
	"github.com/datasance/edgelet/internal/cli/output"
	"github.com/datasance/edgelet/internal/cli/run"
)

// Result carries config patch outcome.
type Result struct {
	Human string
	Data  map[string]interface{}
}

// Patch GETs current config, PATCHes keys, and formats the mutation output.
func Patch(client run.EdgeletAPIClient, setMap map[string]interface{}) (human string, data map[string]interface{}, err error) {
	if client == nil {
		return "", nil, run.NewCLIError(run.CodeInternal, "edgeletapi client is nil", nil)
	}
	payload := map[string]interface{}{"set": setMap}
	before, _ := client.Request("GET", "/v1/system/config", nil)
	after, reqErr := client.Request("PATCH", "/v1/system/config", payload)
	if reqErr != nil {
		return "", nil, run.MapAPIError(reqErr)
	}
	return output.FormatConfigMutationOutput(setMap, before, after), after, nil
}

// HasRejections reports whether a config PATCH response includes rejected keys.
func HasRejections(data map[string]interface{}) bool {
	if len(data) == 0 {
		return false
	}
	errorMap, _ := data["errorMap"].(map[string]interface{})
	return len(errorMap) > 0
}

// ApplySetMap validates and PATCHes the provided config keys.
func ApplySetMap(client run.EdgeletAPIClient, setMap map[string]interface{}) (*Result, error) {
	if len(setMap) == 0 {
		return nil, run.NewCLIError(run.CodeInvalidArgument, "at least one config flag is required (see edgelet config --help)", nil)
	}
	human, data, err := Patch(client, setMap)
	if err != nil {
		return nil, err
	}
	return &Result{Human: human, Data: data}, nil
}
