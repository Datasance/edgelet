package runtimecmd

import (
	"time"

	"github.com/eclipse-iofog/edgelet/internal/cli/client"
	"github.com/eclipse-iofog/edgelet/internal/cli/output"
	"github.com/eclipse-iofog/edgelet/internal/cli/run"
)

const (
	drainRoute              = "/v1/runtime/drain"
	drainHTTPOverhead       = 15 * time.Second
	defaultDrainTimeoutSecs = 90
)

// DrainHTTPClientTimeout returns the HTTP client budget for a runtime drain call.
// Must cover server drain budget plus overhead (DefaultRequestTimeout is only 60s).
func DrainHTTPClientTimeout(timeoutSeconds int) time.Duration {
	if timeoutSeconds <= 0 {
		timeoutSeconds = defaultDrainTimeoutSecs
	}
	return time.Duration(timeoutSeconds)*time.Second + drainHTTPOverhead
}

func configureDrainHTTPTimeout(apiClient run.EdgeletAPIClient, timeoutSeconds int) {
	c, ok := apiClient.(*client.Client)
	if !ok {
		return
	}
	c.SetRequestTimeout(DrainHTTPClientTimeout(timeoutSeconds))
}

// DrainResult carries runtime drain outcome from EdgeletAPI.
type DrainResult struct {
	Human string
	Data  map[string]any
}

// Drain requests a coordinated microservice drain before data-plane containerd stop.
func Drain(client run.EdgeletAPIClient, timeoutSeconds int) (*DrainResult, error) {
	if client == nil {
		return nil, run.NewCLIError(run.CodeInternal, "edgeletapi client is nil", nil)
	}
	configureDrainHTTPTimeout(client, timeoutSeconds)
	payload := map[string]any{}
	if timeoutSeconds > 0 {
		payload["timeoutSeconds"] = timeoutSeconds
	}
	data, err := client.Request("POST", drainRoute, payload)
	if err != nil {
		return nil, run.MapAPIError(err)
	}
	human := output.FormatEdgeletAPIHuman(drainRoute, data)
	return &DrainResult{Human: human, Data: data}, nil
}
