package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/datasance/edgelet/internal/buildmeta"
	"github.com/datasance/edgelet/internal/utils/logging"
	"github.com/datasance/edgelet/internal/version"
)

const (
	versionHandlerModuleName = "Version Api Handler"
)

// VersionHandler handles version API requests
type VersionHandler struct {
	// Version will be injected when available
}

// HandleVersion handles GET /v1/system/version.
func (h *VersionHandler) HandleVersion(w http.ResponseWriter, r *http.Request) {
	logging.LogDebug(versionHandlerModuleName, "Processing version request")

	if r.Method != http.MethodGet {
		logging.LogError(versionHandlerModuleName, "Request method not allowed", nil)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	buildInfo := version.GetBuildInfo()
	versionMap := map[string]any{
		"version":                buildInfo["version"],
		"buildTime":              buildInfo["buildTime"],
		"gitCommit":              buildInfo["gitCommit"],
		"embeddedEngine":         buildmeta.HasEmbeddedEngine(),
		"allowedContainerEngine": buildmeta.AllowedEnginesCSV(),
		"allowedEngines":         buildmeta.AllowedEngines(),
	}

	jsonData, err := json.Marshal(versionMap)
	if err != nil {
		logging.LogError(versionHandlerModuleName, "Failed to marshal version", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if _, werr := w.Write(jsonData); werr != nil {
		logging.LogWarn(versionHandlerModuleName, fmt.Sprintf("Failed to write response: %v", werr))
	}
	logging.LogDebug(versionHandlerModuleName, "Finished processing version request")
}
