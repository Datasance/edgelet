package run

import "github.com/datasance/edgelet/internal/cli/client"

// V3Client is the LocalAPI v1 client surface used by CLI commands.
type V3Client interface {
	RequestV3(method, path string, requestBody interface{}) (map[string]interface{}, error)
	RequestV3MultipartFile(method, path, fileField, filePath string, fields map[string]string) (map[string]interface{}, error)
	IsDaemonRunning() bool
}

// ClientFactory creates a LocalAPI client.
type ClientFactory func() V3Client

// DefaultClientFactory returns the production LocalAPI client.
func DefaultClientFactory() V3Client {
	return client.New()
}
