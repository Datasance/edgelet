package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/eclipse-iofog/agent/internal/config"
	"github.com/eclipse-iofog/agent/internal/gps"
	"github.com/eclipse-iofog/agent/internal/utils/logging"
)

const (
	gpsHandlerModuleName = "GPS API Handler"
)

// GPSHandler handles GPS API requests
type GPSHandler struct{}

// NewGPSHandler creates a new GPS handler
func NewGPSHandler() *GPSHandler {
	return &GPSHandler{}
}

// HandleGetGPS handles GET /v2/gps
func (h *GPSHandler) HandleGetGPS(w http.ResponseWriter, _ *http.Request) {
	logging.LogDebug(gpsHandlerModuleName, "Handling GET /v2/gps")

	gpsManager := gps.GetInstance()
	status := gpsManager.GetStatus()
	cfg := config.GetInstance()

	// Get actual coordinates and mode from config
	coordinates := cfg.GPSCoordinates
	if coordinates == "" {
		coordinates = "0.00000,0.00000"
	}
	mode := cfg.GPSMode
	if mode == "" {
		mode = "OFF"
	}

	response := map[string]interface{}{
		"status":      status.GetHealthStatus(),
		"coordinates": coordinates,
		"mode":        mode,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		logging.LogError(gpsHandlerModuleName, "Failed to encode GPS response", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}
