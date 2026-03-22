package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/eclipse-iofog/agent/internal/config"
	"github.com/eclipse-iofog/agent/internal/fieldagent"
	"github.com/eclipse-iofog/agent/internal/utils/logging"
)

const (
	provisionHandlerModuleName = "Provision Api Handler"
)

// ProvisionHandler handles provision API requests
type ProvisionHandler struct{}

// NewProvisionHandler creates a new provision handler
func NewProvisionHandler() *ProvisionHandler {
	return &ProvisionHandler{}
}

// ProvisionRequest represents the request body
type ProvisionRequest struct {
	ProvisioningKey string `json:"provisioning-key"`
}

// ProvisionResponse represents the response body
type ProvisionResponse struct {
	Status     string `json:"status"`
	UUID       string `json:"uuid,omitempty"`
	PrivateKey string `json:"privateKey,omitempty"`
	Namespace  string `json:"namespace,omitempty"`
	Message    string `json:"message,omitempty"`
}

// HandleProvision handles POST /v2/provision
func (h *ProvisionHandler) HandleProvision(w http.ResponseWriter, r *http.Request) {
	logging.LogDebug(provisionHandlerModuleName, "Processing provision request")

	if r.Method != http.MethodPost {
		logging.LogError(provisionHandlerModuleName, "Request method not allowed", nil)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Validate content type
	contentType := r.Header.Get("Content-Type")
	if contentType != "application/json" {
		logging.LogError(provisionHandlerModuleName, "Invalid content type: "+contentType, nil)
		http.Error(w, "Content-Type must be application/json", http.StatusBadRequest)
		return
	}

	// Read request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		logging.LogError(provisionHandlerModuleName, "Failed to read request body", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Parse JSON
	var req ProvisionRequest
	if err := json.Unmarshal(body, &req); err != nil {
		logging.LogError(provisionHandlerModuleName, "Failed to parse JSON", err)
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Validate provisioning key
	if req.ProvisioningKey == "" {
		logging.LogError(provisionHandlerModuleName, "Missing provisioning-key", nil)
		http.Error(w, "Missing required property 'provisioning-key'", http.StatusBadRequest)
		return
	}

	// Call FieldAgent.Provision
	fa := fieldagent.GetInstance()
	if err := fa.Provision(req.ProvisioningKey); err != nil {
		logging.LogError(provisionHandlerModuleName, "Provisioning failed", err)
		response := ProvisionResponse{
			Status:  "failed",
			Message: err.Error(),
		}
		jsonData, _ := json.Marshal(response)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		if _, werr := w.Write(jsonData); werr != nil {
			logging.LogWarn(provisionHandlerModuleName, fmt.Sprintf("Failed to write response: %v", werr))
		}
		return
	}

	// Get provisioned info from config
	cfg := config.GetInstance()
	response := ProvisionResponse{
		Status:     "okay",
		UUID:       cfg.IOFogUUID,
		PrivateKey: cfg.PrivateKey,
		Namespace:  cfg.Namespace,
	}

	jsonData, err := json.Marshal(response)
	if err != nil {
		logging.LogError(provisionHandlerModuleName, "Failed to marshal response", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if _, werr := w.Write(jsonData); werr != nil {
		logging.LogWarn(provisionHandlerModuleName, fmt.Sprintf("Failed to write response: %v", werr))
	}
	logging.LogDebug(provisionHandlerModuleName, "Finished processing provision request")
}
