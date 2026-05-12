package cli

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/eclipse-iofog/agent/internal/utils"
)

const (
	localAPIBaseURL = "https://localhost:54321"
)

// Client is a client for communicating with the Local API
type Client struct {
	baseURL        string
	unixSocketPath string
	token          string
}

// V3APIError is a structured LocalAPI v3 error.
type V3APIError struct {
	StatusCode int
	Code       string
	Message    string
}

func (e *V3APIError) Error() string {
	if e == nil {
		return ""
	}
	if e.Code == "" {
		return fmt.Sprintf("request failed with status %d: %s", e.StatusCode, e.Message)
	}
	return fmt.Sprintf("%s (%d): %s", e.Code, e.StatusCode, e.Message)
}

// NewClient creates a new Local API client
func NewClient() *Client {
	return &Client{
		baseURL:        localAPIBaseURL,
		unixSocketPath: filepath.Join(utils.VarRun, "iofog-agentd.sock"),
		token:          readAccessToken(),
	}
}

// IsDaemonRunning checks if the daemon is running
// First checks PID file, then tries Local API connection
func (c *Client) IsDaemonRunning() bool {
	// First check PID file (faster and more reliable)
	if utils.IsAnotherInstanceRunning() {
		return true
	}

	// Fallback: try to connect to Local API via v3 status endpoint.
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, c.baseURL+"/v3/system/status", nil)
	if err != nil {
		return false
	}
	req.Header.Set("Accept", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.doV3Request(req)
	if err != nil {
		return false
	}
	if err := resp.Body.Close(); err != nil {
		_ = err // best-effort close for health check
	}
	return resp.StatusCode == http.StatusOK
}

// RequestV3 sends a typed request to LocalAPI v3 and returns JSON response map when possible.
func (c *Client) RequestV3(method, path string, requestBody interface{}) (map[string]interface{}, error) {
	url := c.baseURL + path

	var body io.Reader
	if requestBody != nil {
		data, err := json.Marshal(requestBody)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		body = bytes.NewBuffer(data)
	}

	req, err := http.NewRequestWithContext(context.Background(), method, url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if requestBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.doV3Request(req)
	if err != nil {
		if strings.Contains(err.Error(), "connection refused") || strings.Contains(err.Error(), "connect: connection refused") {
			return nil, fmt.Errorf("Local API is not accessible. The daemon may be starting up or the Local API service is not running. Error: %w", err)
		}
		return nil, fmt.Errorf("failed to send v3 request: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	var envelope struct {
		Success bool                   `json:"success"`
		Data    map[string]interface{} `json:"data"`
		Error   struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}

	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &envelope); err == nil && (envelope.Success || envelope.Error.Code != "" || envelope.Error.Message != "") {
			if resp.StatusCode < 200 || resp.StatusCode >= 300 || !envelope.Success {
				return nil, &V3APIError{
					StatusCode: resp.StatusCode,
					Code:       envelope.Error.Code,
					Message:    envelope.Error.Message,
				}
			}
			if envelope.Data == nil {
				return map[string]interface{}{}, nil
			}
			return envelope.Data, nil
		}
	}

	// Backward-compatible fallback for legacy/non-enveloped responses.
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &V3APIError{
			StatusCode: resp.StatusCode,
			Code:       "HTTP_ERROR",
			Message:    string(raw),
		}
	}
	if len(raw) == 0 {
		return map[string]interface{}{}, nil
	}
	var result map[string]interface{}
	if err := json.Unmarshal(raw, &result); err != nil {
		return map[string]interface{}{"raw": string(raw)}, nil
	}
	return result, nil
}

func (c *Client) doV3Request(req *http.Request) (*http.Response, error) {
	// Prefer unix socket transport for admin/CLI path.
	if resp, err := c.doViaUnixSocket(req, c.unixSocketPath); err == nil {
		return resp, nil
	}
	httpClient := &http.Client{
		Timeout:   15 * time.Second,
		Transport: c.tlsTransport(),
	}
	return httpClient.Do(req)
}

func (c *Client) tlsTransport() *http.Transport {
	caPath := filepath.Join(utils.GetConfigDir(), "localapi-ca.crt")
	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		pool = x509.NewCertPool()
	}
	if pemData, readErr := os.ReadFile(caPath); readErr == nil {
		_ = pool.AppendCertsFromPEM(pemData)
	}
	return &http.Transport{
		TLSClientConfig: &tls.Config{
			RootCAs:    pool,
			MinVersion: tls.VersionTLS12,
		},
	}
}

func (c *Client) doViaUnixSocket(req *http.Request, socketPath string) (*http.Response, error) {
	if socketPath == "" {
		return nil, fmt.Errorf("socket path is empty")
	}
	if _, err := os.Stat(socketPath); err != nil {
		return nil, err
	}
	clone := req.Clone(req.Context())
	clone.URL.Scheme = "http"
	clone.URL.Host = "unix"
	clone.RequestURI = ""

	httpClient := &http.Client{
		Timeout: 15 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "unix", socketPath)
			},
		},
	}
	return httpClient.Do(clone)
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
