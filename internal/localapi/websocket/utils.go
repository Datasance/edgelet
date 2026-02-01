package websocket

import "strings"

// extractIDFromPath extracts the container ID from the URL path
func extractIDFromPath(urlPath, prefix string) (string, error) {
	if !strings.HasPrefix(urlPath, prefix) {
		return "", nil
	}
	
	id := strings.TrimPrefix(urlPath, prefix)
	// Remove query parameters if any
	if idx := strings.Index(id, "?"); idx != -1 {
		id = id[:idx]
	}
	
	id = strings.TrimSpace(id)
	if id == "" {
		return "", nil
	}
	
	return id, nil
}
