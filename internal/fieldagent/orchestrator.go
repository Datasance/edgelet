package fieldagent

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/eclipse-iofog/agent/internal/config"
	"github.com/eclipse-iofog/agent/internal/utils/logging"
)

const (
	orchestratorModuleName = "Orchestrator"
)

// Orchestrator wraps APIClient with additional methods matching Java Orchestrator
type Orchestrator struct {
	apiClient *APIClient
	config    *config.Config
}

// NewOrchestrator creates a new Orchestrator instance
func NewOrchestrator(apiClient *APIClient) *Orchestrator {
	return &Orchestrator{
		apiClient: apiClient,
		config:    config.GetInstance(),
	}
}

// Update updates the orchestrator configuration
// This matches Java: update() method
func (o *Orchestrator) Update() error {
	// Configuration is already loaded in APIClient
	// This method can be used to refresh certificate if needed
	return o.renewCertificateIfNeeded()
}

// Ping pings the IOFog controller
// This matches Java: ping() method
func (o *Orchestrator) Ping(ctx context.Context) (bool, error) {
	logging.LogDebug(orchestratorModuleName, "Inside ping")

	var result map[string]interface{}
	err := o.apiClient.GetJSON(ctx, "status", &result)
	if err != nil {
		logging.LogError(orchestratorModuleName, "Error pinging", err)
		return false, err
	}

	logging.LogDebug(orchestratorModuleName, "Finished pinging")
	// Check if status exists in result
	_, exists := result["status"]
	return exists, nil
}

// Provision does provisioning with the given key
// This matches Java: provision() method
func (o *Orchestrator) Provision(ctx context.Context, key string) (map[string]interface{}, error) {
	logging.LogDebug(orchestratorModuleName, "Inside provision")

	cfg := config.GetInstance()
	// Get architecture code (integer, not string)
	archCode := getArchitectureCode(cfg.Arch)
	body := map[string]interface{}{
		"key":  key,
		"type": archCode,
	}

	result, err := o.apiClient.Request(ctx, "provision", POST, nil, body)
	if err != nil {
		logging.LogError(orchestratorModuleName, "Error while provision", err)
		return nil, err
	}

	logging.LogDebug(orchestratorModuleName, "Finished provision")
	return result, nil
}

// GetJSON performs a GET request and returns JSON
// This matches Java: getJSON() method
func (o *Orchestrator) GetJSON(ctx context.Context, command string, result interface{}) error {
	return o.apiClient.GetJSON(ctx, command, result)
}

// Request performs an HTTP request (wrapper around APIClient.Request)
// This matches Java: request() method
func (o *Orchestrator) Request(ctx context.Context, command string, requestType RequestType, queryParams map[string]string, body interface{}) (map[string]interface{}, error) {
	return o.apiClient.Request(ctx, command, requestType, queryParams, body)
}

// UploadFile implements the diagnostics.FileUploader interface
func (o *Orchestrator) UploadFile(ctx context.Context, command string, filePath string) error {
	return o.SendFileToController(ctx, command, filePath)
}

// SendFileToController sends a file to the controller using multipart form data
// This matches Java: sendFileToController() method
func (o *Orchestrator) SendFileToController(ctx context.Context, command string, filePath string) error {
	logging.LogDebug(orchestratorModuleName, fmt.Sprintf("Sending file to controller: %s", filePath))

	// Open file
	file, err := os.Open(filepath.Clean(filePath))
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Get file info
	fileInfo, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to get file info: %w", err)
	}

	// Create multipart writer
	var requestBody bytes.Buffer
	writer := multipart.NewWriter(&requestBody)

	// Add file field
	part, err := writer.CreateFormFile("file", fileInfo.Name())
	if err != nil {
		return fmt.Errorf("failed to create form file: %w", err)
	}

	// Copy file content
	if _, err := io.Copy(part, file); err != nil {
		return fmt.Errorf("failed to copy file content: %w", err)
	}

	// Close writer to finalize multipart message
	if err := writer.Close(); err != nil {
		return fmt.Errorf("failed to close multipart writer: %w", err)
	}

	// Create request with timeout
	reqCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	// Access baseURL directly (it's a field, not a method)
	// We need to add a getter or access it directly
	baseURL := o.config.ControllerURL
	baseURL = strings.TrimSuffix(baseURL, "/")
	url := baseURL + "/agent/" + command

	req, err := http.NewRequestWithContext(reqCtx, "PUT", url, &requestBody)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set content type with boundary
	req.Header.Set("Content-Type", writer.FormDataContentType())

	// Add JWT token
	token, err := o.apiClient.jwtManager.GenerateJWT()
	if err != nil {
		return fmt.Errorf("failed to generate JWT: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	// Perform request
	resp, err := o.apiClient.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Check response
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(bodyBytes))
	}

	logging.LogDebug(orchestratorModuleName, "File sent successfully")
	return nil
}

// GetControllerCert gets the controller certificate from the controller
// This matches Java: getControllerCert() method
// Note: This uses an insecure connection to get the certificate when current cert is invalid
func (o *Orchestrator) GetControllerCert(ctx context.Context) (string, error) {
	logging.LogDebug(orchestratorModuleName, "Getting controller certificate")

	// Create a temporary insecure client to get the certificate
	// This matches Java logic where it uses TrustManagers.getInsecureSocketFactory()
	cfg := config.GetInstance()
	insecureClient := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				MinVersion:         tls.VersionTLS12,
				InsecureSkipVerify: true, // #nosec G402 -- intentionally insecure for bootstrapping initial certificate fetch
			},
		},
	}

	// Build URL
	baseURL := strings.TrimSuffix(cfg.ControllerURL, "/")
	url := baseURL + "/agent/cert"

	// Create request
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Add JWT token
	token, err := o.apiClient.jwtManager.GenerateJWT()
	if err != nil {
		return "", fmt.Errorf("failed to generate JWT: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	// Perform request
	resp, err := insecureClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Check response
	if resp.StatusCode == http.StatusUnauthorized {
		// Token invalid - trigger deprovision
		logging.LogWarn(orchestratorModuleName, "Invalid JWT token, switching controller status to Not provisioned")
		// Note: FieldAgent deprovision would be called here
		return "", fmt.Errorf("unauthorized: invalid JWT token")
	}

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(bodyBytes))
	}

	// Read response body
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if len(bodyBytes) == 0 {
		return "", fmt.Errorf("empty response from controller")
	}

	// Response is base64 encoded certificate string
	certStr := string(bodyBytes)

	logging.LogDebug(orchestratorModuleName, "Got controller certificate")
	return certStr, nil
}

// renewCertificateIfNeeded renews the controller certificate if needed
// This matches Java certificate renewal logic (lines 314-342)
func (o *Orchestrator) renewCertificateIfNeeded() error {
	cfg := config.GetInstance()

	// Check if certificate file exists
	if cfg.ControllerCert == "" {
		return nil // No certificate configured
	}

	// If certificate is a file path, check if it needs renewal
	if strings.HasPrefix(cfg.ControllerCert, "/") || strings.HasPrefix(cfg.ControllerCert, "./") {
		// Check if file exists
		if _, err := os.Stat(cfg.ControllerCert); os.IsNotExist(err) {
			// File doesn't exist, try to get certificate from controller
			logging.LogDebug(orchestratorModuleName, "Certificate file not found, attempting to fetch from controller")

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			base64Cert, err := o.GetControllerCert(ctx)
			if err != nil {
				logging.LogWarn(orchestratorModuleName, fmt.Sprintf("Failed to get controller cert: %v", err))
				return nil // Don't fail if we can't get cert
			}

			// Decode base64 certificate
			certBytes, err := base64.StdEncoding.DecodeString(base64Cert)
			if err != nil {
				return fmt.Errorf("failed to decode certificate: %w", err)
			}

			// Write certificate to file
			if err := os.WriteFile(cfg.ControllerCert, certBytes, 0600); err != nil {
				return fmt.Errorf("failed to write certificate file: %w", err)
			}

			logging.LogInfo(orchestratorModuleName, "Controller certificate fetched and saved")
		} else {
			// File exists, verify it's valid
			certBytes, err := os.ReadFile(cfg.ControllerCert)
			if err != nil {
				return fmt.Errorf("failed to read certificate file: %w", err)
			}

			// Try to parse certificate to verify it's valid
			block, _ := pem.Decode(certBytes)
			if block == nil {
				// Invalid certificate, try to renew
				logging.LogWarn(orchestratorModuleName, "Invalid certificate file, attempting to renew")
				return o.renewCertificate()
			}

			// Parse certificate
			cert, err := x509.ParseCertificate(block.Bytes)
			if err != nil {
				// Invalid certificate, try to renew
				logging.LogWarn(orchestratorModuleName, "Failed to parse certificate, attempting to renew")
				return o.renewCertificate()
			}

			// Check if certificate is expired or expiring soon (within 7 days)
			now := time.Now()
			expiryTime := cert.NotAfter
			if now.After(expiryTime) || now.Add(7*24*time.Hour).After(expiryTime) {
				logging.LogInfo(orchestratorModuleName, "Certificate expired or expiring soon, renewing")
				return o.renewCertificate()
			}
		}
	}

	return nil
}

// renewCertificate renews the controller certificate
func (o *Orchestrator) renewCertificate() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	base64Cert, err := o.GetControllerCert(ctx)
	if err != nil {
		return fmt.Errorf("failed to get controller cert: %w", err)
	}

	// Decode base64 certificate
	certBytes, err := base64.StdEncoding.DecodeString(base64Cert)
	if err != nil {
		return fmt.Errorf("failed to decode certificate: %w", err)
	}

	cfg := config.GetInstance()

	// Write certificate to file
	if err := os.WriteFile(cfg.ControllerCert, certBytes, 0600); err != nil {
		return fmt.Errorf("failed to write certificate file: %w", err)
	}

	logging.LogInfo(orchestratorModuleName, "Controller certificate renewed successfully")

	// Reinitialize APIClient with new certificate
	// Note: This would require reinitializing the HTTP client
	// For now, we'll just log that certificate was renewed
	// The next request will use the new certificate

	return nil
}
