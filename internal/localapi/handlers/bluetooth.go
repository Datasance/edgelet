package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/eclipse-iofog/agent/internal/utils/logging"
)

const (
	bluetoothHandlerModuleName = "Bluetooth API Handler"
)

// BluetoothHandler handles Bluetooth API requests
type BluetoothHandler struct{}

// NewBluetoothHandler creates a new Bluetooth handler
func NewBluetoothHandler() *BluetoothHandler {
	return &BluetoothHandler{}
}

// HandleGetBluetooth handles GET /v2/bluetooth
func (h *BluetoothHandler) HandleGetBluetooth(w http.ResponseWriter, _ *http.Request) {
	logging.LogDebug(bluetoothHandlerModuleName, "Handling GET /v2/bluetooth")

	// Bluetooth functionality is not fully implemented in the Java codebase
	// This is a placeholder endpoint
	response := map[string]interface{}{
		"devices": []interface{}{},
		"status":  "not_implemented",
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		logging.LogError(bluetoothHandlerModuleName, "Failed to encode Bluetooth response", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}
