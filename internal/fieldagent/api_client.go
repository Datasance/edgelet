package fieldagent

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/datasance/edgelet/internal/auth"
	"github.com/datasance/edgelet/internal/config"
	"github.com/datasance/edgelet/internal/utils"
	"github.com/datasance/edgelet/internal/utils/logging"
)

const (
	connectionTimeout = 10 * time.Second
	socketTimeout     = 5 * time.Minute
	requestTimeout    = 5 * time.Minute
	maxRetries        = 3
	initialBackoff    = 1 * time.Second
	maxBackoff        = 30 * time.Second
)

// APIClient handles all HTTP communication with the controller
type APIClient struct {
	baseURL        string
	httpClient     *http.Client
	jwtManager     *auth.JWTManager
	controllerCert *x509.Certificate
}

// NewAPIClient creates a new API client for controller communication
func NewAPIClient() (*APIClient, error) {
	cfg := config.GetInstance()

	// Load config if not already loaded (CLI mode)
	if cfg.GetYamlConfig() == nil {
		configPath := utils.ConfigYAMLPath
		if err := config.LoadConfig(configPath); err != nil {
			// Log warning but continue - config might have defaults
			logging.LogWarn("Field Agent", fmt.Sprintf("Could not load config from %s: %v. Using defaults.", configPath, err))
		}
	}

	// Create HTTP client with timeout
	httpClient := &http.Client{
		Timeout: requestTimeout,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				MinVersion:         tls.VersionTLS12,
				InsecureSkipVerify: !cfg.SecureMode, // #nosec G402 -- controlled by SecureMode config; false in production
			},
		},
	}

	// Load controller certificate if configured
	var controllerCert *x509.Certificate
	if cfg.ControllerCert != "" {
		var cert *x509.Certificate
		var err error

		// Check if it's a file path (starts with /)
		if strings.HasPrefix(cfg.ControllerCert, "/") || strings.HasPrefix(cfg.ControllerCert, "./") {
			// Load from file
			certs, err := auth.LoadCertificatesFromFile(cfg.ControllerCert)
			if err != nil {
				// If file doesn't exist or can't be read, log warning but continue
				logging.LogError("Field Agent", fmt.Sprintf("Could not load certificate from file %s: %v. Continuing without certificate.", cfg.ControllerCert, err), err)
			} else if len(certs) > 0 {
				cert = certs[0]
			}
		} else {
			// Try loading as base64 first
			cert, err = auth.LoadCertificateFromBase64(cfg.ControllerCert)
			if err != nil {
				// Try loading as PEM string directly
				cert, err = auth.LoadCertificateFromPEM([]byte(cfg.ControllerCert))
				if err != nil {
					logging.LogError("Field Agent", fmt.Sprintf("Could not load certificate: %v. Continuing without certificate.", err), err)
				}
			}
		}

		if cert != nil {
			controllerCert = cert

			// Update transport with certificate
			certPool := x509.NewCertPool()
			certPool.AddCert(cert)
			httpClient.Transport = &http.Transport{
				TLSClientConfig: &tls.Config{
					MinVersion: tls.VersionTLS12,
					RootCAs:    certPool,
				},
			}
		}
	}

	// Normalize baseURL - remove trailing slash if present to avoid double slashes
	baseURL := strings.TrimSuffix(cfg.ControllerURL, "/")

	return &APIClient{
		baseURL:        baseURL,
		httpClient:     httpClient,
		jwtManager:     auth.GetJWTManager(),
		controllerCert: controllerCert,
	}, nil
}

// RequestType represents HTTP request types
type RequestType string

const (
	GET    RequestType = "GET"
	POST   RequestType = "POST"
	PUT    RequestType = "PUT"
	PATCH  RequestType = "PATCH"
	DELETE RequestType = "DELETE"
)

// Request performs an HTTP request to the controller with retry logic
func (c *APIClient) Request(ctx context.Context, command string, requestType RequestType, queryParams map[string]string, body any) (map[string]any, error) {
	var lastErr error
	backoff := initialBackoff

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// Wait before retry with exponential backoff
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
			backoff = time.Duration(float64(backoff) * 2)
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}

		result, err := c.doRequest(ctx, command, requestType, queryParams, body)
		if err == nil {
			return result, nil
		}

		lastErr = err

		// Don't retry on certain errors (unauthorized, bad request, etc.)
		if !shouldRetry(err) {
			return nil, err
		}
	}

	return nil, fmt.Errorf("request failed after %d attempts: %w", maxRetries+1, lastErr)
}

// doRequest performs a single HTTP request to the controller
func (c *APIClient) doRequest(ctx context.Context, command string, requestType RequestType, queryParams map[string]string, body any) (map[string]any, error) {
	url := c.baseURL + "/agent/" + command

	// Use configurable timeout (edge-friendly defaults: 30s)
	timeoutSec := config.GetInstance().ControllerRequestTimeoutSeconds
	if timeoutSec < 5 {
		timeoutSec = 30
	}
	reqCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
	defer cancel()

	var reqBody io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewBuffer(jsonData)
	}

	req, err := http.NewRequestWithContext(reqCtx, string(requestType), url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add query parameters
	if queryParams != nil {
		q := req.URL.Query()
		for k, v := range queryParams {
			q.Add(k, v)
		}
		req.URL.RawQuery = q.Encode()
	}

	// Generate request ID for logging
	requestID := fmt.Sprintf("%d", time.Now().UnixNano())

	// Log request
	logging.LogDebug("Orchestrator", fmt.Sprintf("(%s) %s %s", requestID, string(requestType), req.URL.String()))

	// Set headers
	req.Header.Set("Content-Type", "application/json")

	// Add Request-Id header
	req.Header.Set("Request-Id", requestID)

	// Add JWT token only for non-provisioning requests
	// Provisioning doesn't require JWT since agent doesn't have private key yet
	if command != "provision" {
		cfg := config.GetInstance()

		token, err := c.jwtManager.GenerateJWT()
		if err != nil {
			// If JWT generation fails and we're not provisioned, that's expected
			// Only fail if we should have a private key (i.e., we're provisioned)
			if cfg.IOFogUUID != "" && cfg.PrivateKey != "" {
				return nil, fmt.Errorf("failed to generate JWT: %w", err)
			}
			// Otherwise, continue without JWT (for unprovisioned agents)
			logging.LogDebug("Field Agent", "Skipping JWT for unprovisioned agent")
		} else {
			req.Header.Set("Authorization", "Bearer "+token)
		}
	}

	// Perform request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	// Handle status codes
	switch resp.StatusCode {
	case http.StatusOK:
		var result map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return nil, fmt.Errorf("failed to decode response: %w", err)
		}
		return result, nil
	case http.StatusNoContent: // 204 - No Content (success with empty body)
		return make(map[string]any), nil
	case http.StatusUnauthorized:
		// Token invalid - trigger deprovision
		return nil, errors.New("unauthorized: invalid JWT token")
	case http.StatusNotFound:
		return nil, errors.New("not found: controller endpoint not found")
	case http.StatusBadRequest:
		return nil, errors.New("bad request: invalid request")
	case http.StatusForbidden:
		return nil, errors.New("forbidden: access forbidden")
	case http.StatusInternalServerError:
		return nil, errors.New("internal server error")
	default:
		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			return nil, fmt.Errorf("client error: status %d", resp.StatusCode)
		} else if resp.StatusCode >= 500 {
			return nil, fmt.Errorf("server error: status %d", resp.StatusCode)
		}
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
}

// shouldRetry determines if an error should trigger a retry
func shouldRetry(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()

	// Don't retry on client errors (4xx except 408, 429)
	if strings.Contains(errStr, "unauthorized") ||
		strings.Contains(errStr, "forbidden") ||
		strings.Contains(errStr, "not found") ||
		strings.Contains(errStr, "bad request") {
		return false
	}

	// Retry on network errors, timeouts, and server errors
	return strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "connection") ||
		strings.Contains(errStr, "server error") ||
		strings.Contains(errStr, "internal server error")
}

// Ping pings the controller to check connectivity
func (c *APIClient) Ping(ctx context.Context) (bool, error) {
	// Ping uses a special endpoint: /api/v3/status (not /api/v3/agent/status)
	url := c.baseURL + "/status"

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return false, fmt.Errorf("failed to create ping request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")

	// Add JWT token for ping
	cfg := config.GetInstance()
	if cfg.IOFogUUID != "" && cfg.PrivateKey != "" {
		jwt, err := c.jwtManager.GenerateJWT()
		if err != nil {
			return false, fmt.Errorf("failed to generate JWT for ping: %w", err)
		}
		if jwt != "" {
			req.Header.Set("Authorization", "Bearer "+jwt)
		}
	}

	// Execute request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("ping request failed: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	// Check status code
	if resp.StatusCode == http.StatusNotFound {
		return false, errors.New("not found: controller endpoint not found")
	}
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("ping failed with status %d", resp.StatusCode)
	}

	// Parse response
	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, fmt.Errorf("failed to decode ping response: %w", err)
	}

	_, exists := result["status"]
	return exists, nil
}

// GetJSON performs a GET request and unmarshals JSON response
func (c *APIClient) GetJSON(ctx context.Context, command string, result any) error {
	resp, err := c.Request(ctx, command, GET, nil, nil)
	if err != nil {
		return err
	}

	// Convert map to JSON and unmarshal
	jsonData, err := json.Marshal(resp)
	if err != nil {
		return fmt.Errorf("failed to marshal response: %w", err)
	}

	if err := json.Unmarshal(jsonData, result); err != nil {
		return fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return nil
}

// PostJSON performs a POST request with JSON body
func (c *APIClient) PostJSON(ctx context.Context, command string, body any, result any) error {
	resp, err := c.Request(ctx, command, POST, nil, body)
	if err != nil {
		return err
	}

	if result != nil {
		jsonData, err := json.Marshal(resp)
		if err != nil {
			return fmt.Errorf("failed to marshal response: %w", err)
		}

		if err := json.Unmarshal(jsonData, result); err != nil {
			return fmt.Errorf("failed to unmarshal response: %w", err)
		}
	}

	return nil
}

// PutJSON performs a PUT request with JSON body
func (c *APIClient) PutJSON(ctx context.Context, command string, body any) error {
	_, err := c.Request(ctx, command, PUT, nil, body)
	return err
}

// PatchJSON performs a PATCH request with JSON body
func (c *APIClient) PatchJSON(ctx context.Context, command string, body any) error {
	_, err := c.Request(ctx, command, PATCH, nil, body)
	return err
}

// Delete performs a DELETE request
func (c *APIClient) Delete(ctx context.Context, command string) error {
	_, err := c.Request(ctx, command, DELETE, nil, nil)
	return err
}
