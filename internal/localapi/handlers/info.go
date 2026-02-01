package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/eclipse-iofog/agent-go/internal/config"
	"github.com/eclipse-iofog/agent-go/internal/network"
	"github.com/eclipse-iofog/agent-go/internal/utils/logging"
)

const (
	infoHandlerModuleName = "Info Api Handler"
)

// InfoHandler handles info API requests
type InfoHandler struct {
	// Will be injected with Configuration when needed
}

// HandleInfo handles GET /v2/info
func (h *InfoHandler) HandleInfo(w http.ResponseWriter, r *http.Request) {
	logging.LogDebug(infoHandlerModuleName, "Processing info http request")

	if r.Method != http.MethodGet {
		logging.LogError(infoHandlerModuleName, "Request method not allowed", nil)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get actual config from Configuration
	cfg := config.GetInstance()
	
	// Get IP address from NetworkInterfaceManager
	ipAddress := network.GetInstance().GetCurrentIPAddress()
	if ipAddress == "" {
		ipAddress = "unable to retrieve ip address"
	}
	
	// Get config report (with IP address)
	configReport := cfg.GetConfigReportWithIP(ipAddress)
	
	// Parse config report into map
	infoMap := parseInfoReport(configReport)

	// Convert to JSON
	jsonData, err := json.Marshal(infoMap)
	if err != nil {
		logging.LogError(infoHandlerModuleName, "Failed to marshal info", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(jsonData)
	logging.LogDebug(infoHandlerModuleName, "Finished processing info http request")
}

// parseInfoReport parses an info report string into a map
func parseInfoReport(infoReport string) map[string]string {
	result := make(map[string]string)
	lines := strings.Split(infoReport, "\n")
	
	for _, line := range lines {
		parts := strings.SplitN(line, " : ", 2)
		if len(parts) == 2 {
			key := strings.ToLower(strings.TrimSpace(parts[0]))
			key = strings.ReplaceAll(key, " ", "-")
			
			// Handle special key mappings from Java implementation
			if key == "gps-coordinates(lat,lon)" {
				key = "gps-coordinates"
			} else if key == "developer's-mode" {
				key = "developer-mode"
			}
			
			value := strings.TrimSpace(parts[1])
			result[key] = value
		}
	}
	
	return result
}
