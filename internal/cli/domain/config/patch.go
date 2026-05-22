package config

import (
	"strings"

	"github.com/eclipse-iofog/agent/internal/cli/output"
	"github.com/eclipse-iofog/agent/internal/cli/run"
)

// Result carries config patch outcome.
type Result struct {
	Human string
	Data  map[string]interface{}
}

// Patch GETs current config, PATCHes keys, and formats the mutation output.
func Patch(client run.V3Client, setMap map[string]interface{}) (human string, data map[string]interface{}, err error) {
	if client == nil {
		return "", nil, run.NewCLIError(run.CodeInternal, "localapi client is nil", nil)
	}
	payload := map[string]interface{}{"set": setMap}
	before, _ := client.RequestV3("GET", "/v3/system/config", nil)
	after, reqErr := client.RequestV3("PATCH", "/v3/system/config", payload)
	if reqErr != nil {
		return "", nil, run.MapAPIError(reqErr)
	}
	return output.FormatConfigMutationOutput(setMap, before, after), after, nil
}

// Apply parses config args, rejects the legacy "set" subcommand, and patches config.
func Apply(client run.V3Client, args []string) (*Result, error) {
	if len(args) == 0 {
		return nil, run.NewCLIError(run.CodeInvalidArgument, "usage: iofog-agent config <key> <value> [<key> <value> ...]", nil)
	}
	if strings.EqualFold(strings.TrimSpace(args[0]), "set") {
		return nil, run.NewCLIError(run.CodeInvalidArgument, "config set subcommand is not supported; use key/value pairs directly", nil)
	}
	setMap, parseErr := ParseSetArgs(args)
	if parseErr != nil {
		return nil, run.NewCLIError(run.CodeInvalidArgument, parseErr.Error(), parseErr)
	}
	human, data, err := Patch(client, setMap)
	if err != nil {
		return nil, err
	}
	return &Result{Human: human, Data: data}, nil
}
