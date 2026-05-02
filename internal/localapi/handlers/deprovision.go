package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/eclipse-iofog/agent/internal/fieldagent"
	"github.com/eclipse-iofog/agent/internal/utils/logging"
)

const (
	deprovisionHandlerModuleName = "Deprovision Api Handler"
)

// DeprovisionHandler handles deprovision API requests
type DeprovisionHandler struct{}

// NewDeprovisionHandler creates a new deprovision handler
func NewDeprovisionHandler() *DeprovisionHandler {
	return &DeprovisionHandler{}
}

// DeprovisionResponse represents the response body
type DeprovisionResponse struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

// HandleDeprovision handles DELETE /v2/deprovision (matching Java DeprovisionApiHandler)
func (h *DeprovisionHandler) HandleDeprovision(w http.ResponseWriter, r *http.Request) {
	logging.LogDebug(deprovisionHandlerModuleName, "Processing deprovision request")

	if r.Method != http.MethodDelete {
		logging.LogError(deprovisionHandlerModuleName, "Request method not allowed", nil)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check if already not provisioned
	fa := fieldagent.GetInstance()
	if fa.NotProvisioned() {
		response := DeprovisionResponse{
			Status:  "failed",
			Message: "Failure - not provisioned",
		}
		jsonData, _ := json.Marshal(response)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		if _, werr := w.Write(jsonData); werr != nil {
			logging.LogWarn(deprovisionHandlerModuleName, fmt.Sprintf("Failed to write response: %v", werr))
		}
		return
	}

	// clearCredentials = false: attempt to notify controller before wiping credentials
	if err := fa.Deprovision(false); err != nil {
		logging.LogError(deprovisionHandlerModuleName, "Deprovisioning failed", err)
		response := DeprovisionResponse{
			Status:  "failed",
			Message: err.Error(),
		}
		jsonData, _ := json.Marshal(response)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		if _, werr := w.Write(jsonData); werr != nil {
			logging.LogWarn(deprovisionHandlerModuleName, fmt.Sprintf("Failed to write response: %v", werr))
		}
		return
	}

	response := DeprovisionResponse{
		Status:  "success",
		Message: "Deprovisioned successfully",
	}

	jsonData, err := json.Marshal(response)
	if err != nil {
		logging.LogError(deprovisionHandlerModuleName, "Failed to marshal response", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if _, werr := w.Write(jsonData); werr != nil {
		logging.LogWarn(deprovisionHandlerModuleName, fmt.Sprintf("Failed to write response: %v", werr))
	}
	logging.LogDebug(deprovisionHandlerModuleName, "Finished processing deprovision request")
}
