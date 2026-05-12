package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/eclipse-iofog/agent/internal/statusreporter"
	"github.com/eclipse-iofog/agent/internal/utils/logging"
)

const (
	statusHandlerModuleName = "Status Api Handler"
)

// StatusHandler handles status API requests
type StatusHandler struct {
	// StatusReporter is accessed via singleton
}

// HandleStatus handles GET /v3/system/status
func (h *StatusHandler) HandleStatus(w http.ResponseWriter, r *http.Request) {
	logging.LogDebug(statusHandlerModuleName, "Handle status Api Handler call")

	if r.Method != http.MethodGet {
		logging.LogError(statusHandlerModuleName, "Request method not allowed", nil)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get status report from StatusReporter
	statusReporter := statusreporter.GetInstance()
	statusReport := statusReporter.GetStatusReport()

	// Parse status report into a map
	statusMap := parseStatusReport(statusReport)

	// Convert to JSON
	jsonData, err := json.Marshal(statusMap)
	if err != nil {
		logging.LogError(statusHandlerModuleName, "Failed to marshal status", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if _, werr := w.Write(jsonData); werr != nil {
		logging.LogWarn(statusHandlerModuleName, fmt.Sprintf("Failed to write response: %v", werr))
	}
	logging.LogDebug(statusHandlerModuleName, "Finished status Api Handler call")
}

// parseStatusReport parses a status report string into a map
// This matches the Java implementation which splits by "\n" and " : "
func parseStatusReport(statusReport string) map[string]string {
	result := make(map[string]string)
	lines := strings.Split(statusReport, "\n")

	for _, line := range lines {
		parts := strings.SplitN(line, " : ", 2)
		if len(parts) == 2 {
			key := normalizeReportKey(parts[0])
			if key == "" {
				continue
			}
			value := strings.TrimSpace(parts[1])
			result[key] = value
		}
	}

	return result
}
