package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/eclipse-iofog/agent/internal/utils"
)

const (
	localAPIEndpoint = "http://localhost:54321/v2/commandline"
)

// Client is a client for communicating with the Local API
type Client struct {
	endpoint string
	token    string
}

// NewClient creates a new Local API client
func NewClient() *Client {
	return &Client{
		endpoint: localAPIEndpoint,
		token:    readAccessToken(),
	}
}

// SendCommand sends a command to the daemon via Local API
func (c *Client) SendCommand(command string) (string, error) {
	// Prepare request body
	requestBody := map[string]string{
		"command": command,
	}
	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, c.endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/plain")
	if c.token != "" {
		req.Header.Set("Authorization", c.token)
	}

	// Send request
	httpClient := &http.Client{
		Timeout: 10 * time.Second,
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		// Check if it's a connection error
		if strings.Contains(err.Error(), "connection refused") || strings.Contains(err.Error(), "connect: connection refused") {
			return "", fmt.Errorf("Local API is not accessible. The daemon may be starting up or the Local API service is not running. Error: %w", err)
		}
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	// Check status code
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		// If not JSON, return as-is
		return string(body), nil
	}

	// Extract response field
	if response, ok := result["response"].(string); ok {
		// Replace escaped newlines with actual newlines (matching Java client behavior)
		response = strings.ReplaceAll(response, "\\n", "\n")
		return response, nil
	}

	// If not JSON, return as-is
	return string(body), nil
}

// IsDaemonRunning checks if the daemon is running
// First checks PID file, then tries Local API connection
func (c *Client) IsDaemonRunning() bool {
	// First check PID file (faster and more reliable)
	if utils.IsAnotherInstanceRunning() {
		return true
	}

	// Fallback: try to connect to Local API
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, c.endpoint, bytes.NewBufferString(`{"command":"status"}`))
	if err != nil {
		return false
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", c.token)
	}

	client := &http.Client{
		Timeout: 2 * time.Second, // Short timeout for quick check
	}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	if err := resp.Body.Close(); err != nil {
		_ = err // best-effort close for health check
	}
	return resp.StatusCode == http.StatusOK
}

// readAccessToken reads the access token from file
// Recalculates config path to support dev environment (SNAP_COMMON changes)
func readAccessToken() string {
	// Recalculate config path in case SNAP_COMMON changed (for dev environment)
	configDir := utils.GetConfigDir()
	tokenPath := filepath.Join(configDir, "local-api")
	data, err := os.ReadFile(tokenPath) // #nosec G304 -- path computed from known config directory constant
	if err != nil {
		return ""
	}
	return string(bytes.TrimSpace(data))
}
