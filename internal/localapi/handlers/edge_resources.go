package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/eclipse-iofog/agent-go/internal/fieldagent"
	"github.com/eclipse-iofog/agent-go/internal/utils/logging"
)

const (
	edgeResourcesHandlerModuleName = "Edge Resources Api Handler"
)

// EdgeResourcesHandler handles edge resources API requests
type EdgeResourcesHandler struct{}

// NewEdgeResourcesHandler creates a new edge resources handler
func NewEdgeResourcesHandler() *EdgeResourcesHandler {
	return &EdgeResourcesHandler{}
}

// EdgeResourcesResponse represents the response body
type EdgeResourcesResponse struct {
	Status        string                   `json:"status"`
	EdgeResources []map[string]interface{} `json:"edgeResources,omitempty"`
}

// HandleEdgeResources handles GET /v2/edgeResources
func (h *EdgeResourcesHandler) HandleEdgeResources(w http.ResponseWriter, r *http.Request) {
	logging.LogDebug(edgeResourcesHandlerModuleName, "Processing edge resources request")

	if r.Method != http.MethodGet {
		logging.LogError(edgeResourcesHandlerModuleName, "Request method not allowed", nil)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check if provisioned
	fa := fieldagent.GetInstance()
	if fa.NotProvisioned() || !fa.IsControllerConnected(false) {
		// Return empty response if not provisioned or not connected
		response := EdgeResourcesResponse{
			Status:        "okay",
			EdgeResources: []map[string]interface{}{},
		}
		jsonData, _ := json.Marshal(response)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(jsonData)
		return
	}

	// Get edge resources from FieldAgent
	edgeResources := fa.GetEdgeResources()
	
	response := EdgeResourcesResponse{
		Status:        "okay",
		EdgeResources: edgeResources,
	}

	jsonData, err := json.Marshal(response)
	if err != nil {
		logging.LogError(edgeResourcesHandlerModuleName, "Failed to marshal response", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(jsonData)
	logging.LogDebug(edgeResourcesHandlerModuleName, "Finished processing edge resources request")
}
