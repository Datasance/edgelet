package utils //nolint:revive // legacy package name

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	connectionTimeout = 10 * time.Second
	socketTimeout     = 5 * time.Minute
	requestTimeout    = 5 * time.Minute
)

// HTTPClient is a wrapper around http.Client with retry logic and JWT support
type HTTPClient struct {
	client   *http.Client
	baseURL  string
	jwtToken string
}

// NewHTTPClient creates a new HTTP client
func NewHTTPClient(baseURL string) *HTTPClient {
	return &HTTPClient{
		baseURL: baseURL,
		client: &http.Client{
			Timeout: requestTimeout,
		},
	}
}

// SetJWTToken sets the JWT token for authentication
func (c *HTTPClient) SetJWTToken(token string) {
	c.jwtToken = token
}

// SetControllerCert sets the controller certificate for TLS
func (c *HTTPClient) SetControllerCert(cert *x509.Certificate) error {
	if cert == nil {
		c.client.Transport = http.DefaultTransport
		return nil
	}

	// Create a certificate pool with the controller cert
	certPool := x509.NewCertPool()
	certPool.AddCert(cert)

	// Create TLS config
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
		RootCAs:    certPool,
	}

	// Create transport with TLS config
	transport := &http.Transport{
		TLSClientConfig: tlsConfig,
	}

	c.client.Transport = transport
	return nil
}

// Get performs a GET request with retry logic
func (c *HTTPClient) Get(ctx context.Context, path string) (*http.Response, error) {
	return c.doRequest(ctx, http.MethodGet, path, nil)
}

// Post performs a POST request with retry logic
func (c *HTTPClient) Post(ctx context.Context, path string, body any) (*http.Response, error) {
	return c.doRequest(ctx, http.MethodPost, path, body)
}

// Put performs a PUT request with retry logic
func (c *HTTPClient) Put(ctx context.Context, path string, body any) (*http.Response, error) {
	return c.doRequest(ctx, http.MethodPut, path, body)
}

// Delete performs a DELETE request with retry logic
func (c *HTTPClient) Delete(ctx context.Context, path string) (*http.Response, error) {
	return c.doRequest(ctx, http.MethodDelete, path, nil)
}

// doRequest performs an HTTP request with retry logic
func (c *HTTPClient) doRequest(ctx context.Context, method, path string, body any) (*http.Response, error) {
	url := c.baseURL + path

	var reqBody io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewBuffer(jsonData)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	if c.jwtToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.jwtToken)
	}

	// Perform request with timeout
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	resp, err := c.client.Do(req.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	return resp, nil
}

// GetJSON performs a GET request and unmarshals the JSON response
func (c *HTTPClient) GetJSON(ctx context.Context, path string, result any) error {
	resp, err := c.Get(ctx, path)
	if err != nil {
		return err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
		return fmt.Errorf("failed to decode JSON response: %w", err)
	}

	return nil
}

// PostJSON performs a POST request with JSON body and unmarshals the JSON response
func (c *HTTPClient) PostJSON(ctx context.Context, path string, body any, result any) error {
	resp, err := c.Post(ctx, path, body)
	if err != nil {
		return err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	if result != nil {
		if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
			return fmt.Errorf("failed to decode JSON response: %w", err)
		}
	}

	return nil
}

// Ping performs a ping request to check connectivity
func (c *HTTPClient) Ping(ctx context.Context) (bool, error) {
	var result map[string]any
	err := c.GetJSON(ctx, "status", &result)
	if err != nil {
		return false, err
	}
	_, exists := result["status"]
	return exists, nil
}
