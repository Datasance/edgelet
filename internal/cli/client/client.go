package client

// V3API is the LocalAPI v3 surface used by CLI client helpers.
type V3API interface {
	RequestV3(method, path string, requestBody interface{}) (map[string]interface{}, error)
	RequestV3MultipartFile(method, path, fileField, filePath string, fields map[string]string) (map[string]interface{}, error)
	IsDaemonRunning() bool
}

var _ V3API = (*Client)(nil)
