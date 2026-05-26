package client

// EdgeletAPI is the EdgeletAPI v1 surface used by CLI client helpers.
type EdgeletAPI interface {
	Request(method, path string, requestBody interface{}) (map[string]interface{}, error)
	RequestMultipartFile(method, path, fileField, filePath string, fields map[string]string) (map[string]interface{}, error)
	IsDaemonRunning() bool
}

var _ EdgeletAPI = (*Client)(nil)
