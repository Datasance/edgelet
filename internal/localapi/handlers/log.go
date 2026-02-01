package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/eclipse-iofog/agent-go/internal/config"
	"github.com/eclipse-iofog/agent-go/internal/utils/logging"
)

const (
	logHandlerModuleName = "Log Api Handler"
)

// LogHandler handles log API requests
type LogHandler struct{}

// NewLogHandler creates a new log handler
func NewLogHandler() *LogHandler {
	return &LogHandler{}
}

// LogResponse represents the response body
type LogResponse struct {
	Status string `json:"status"`
	Logs   string `json:"logs,omitempty"`
}

// HandleLog handles GET /v2/log
func (h *LogHandler) HandleLog(w http.ResponseWriter, r *http.Request) {
	logging.LogDebug(logHandlerModuleName, "Processing log request")

	if r.Method != http.MethodGet {
		logging.LogError(logHandlerModuleName, "Request method not allowed", nil)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get logs from log file
	cfg := config.GetInstance()
	logDir := cfg.LogDiskDirectory
	if logDir == "" {
		logDir = "/var/log/iofog-agent"
	}

	// Read latest log file
	logFile := filepath.Join(logDir, "iofog-agent.0.log")
	logs := ""
	
	if file, err := os.Open(logFile); err == nil {
		defer file.Close()
		if data, err := io.ReadAll(file); err == nil {
			// Limit to last 1000 lines to avoid huge responses
			lines := strings.Split(string(data), "\n")
			start := 0
			if len(lines) > 1000 {
				start = len(lines) - 1000
			}
			logs = strings.Join(lines[start:], "\n")
		}
	} else {
		// Log file doesn't exist or can't be read
		logging.LogDebug(logHandlerModuleName, "Log file not found or unreadable: "+logFile)
	}

	response := LogResponse{
		Status: "okay",
		Logs:   logs,
	}

	jsonData, err := json.Marshal(response)
	if err != nil {
		logging.LogError(logHandlerModuleName, "Failed to marshal response", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(jsonData)
	logging.LogDebug(logHandlerModuleName, "Finished processing log request")
}
