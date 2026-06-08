package run

import "github.com/datasance/edgelet/internal/cli/client"

// EdgeletAPIClient is the EdgeletAPI v1 client surface used by CLI commands.
type EdgeletAPIClient interface {
	Request(method, path string, requestBody any) (map[string]any, error)
	RequestMultipartFile(method, path, fileField, filePath string, fields map[string]string) (map[string]any, error)
	IsDaemonRunning() bool
}

// ClientFactory creates a EdgeletAPI client.
type ClientFactory func() EdgeletAPIClient

// DefaultClientFactory returns the production EdgeletAPI client.
func DefaultClientFactory() EdgeletAPIClient {
	return client.New()
}
