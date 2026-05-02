package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/eclipse-iofog/agent/internal/config"
	"github.com/eclipse-iofog/agent/internal/fieldagent"
	"github.com/eclipse-iofog/agent/internal/utils/logging"
)

const (
	configHandlerModuleName = "Config Api Handler"
)

// ConfigHandler handles config API requests
type ConfigHandler struct{}

// NewConfigHandler creates a new config handler
func NewConfigHandler() *ConfigHandler {
	return &ConfigHandler{}
}

// ConfigGetRequest represents the request for GET /v2/config/get
type ConfigGetRequest struct {
	ID string `json:"id"` // Container/microservice ID
}

// ConfigGetResponse represents the response for GET /v2/config/get
type ConfigGetResponse struct {
	Status string `json:"status"`
	Config string `json:"config,omitempty"` // Changed to string to match Java (JSON string, not object)
}

// ConfigSetRequest represents the request for POST /v2/config
type ConfigSetRequest struct {
	DiskLimit            *string `json:"disk-limit,omitempty"`
	DiskDirectory        *string `json:"disk-directory,omitempty"`
	MemoryLimit          *string `json:"memory-limit,omitempty"`
	CPULimit             *string `json:"cpu-limit,omitempty"`
	ControllerURL        *string `json:"controller-url,omitempty"`
	CertDirectory        *string `json:"cert-directory,omitempty"`
	DockerURL            *string `json:"docker-url,omitempty"`
	ContainerEngine      *string `json:"container-engine,omitempty"`
	NetworkAdapter       *string `json:"network-adapter,omitempty"`
	LogsLimit            *string `json:"logs-limit,omitempty"`
	LogsDirectory        *string `json:"logs-directory,omitempty"`
	LogsCount            *string `json:"logs-count,omitempty"`
	StatusFrequency      *string `json:"status-frequency,omitempty"`
	ChangesFrequency     *string `json:"changes-frequency,omitempty"`
	DiagnosticsFrequency *string `json:"diagnostics-frequency,omitempty"`
	DeviceScanFrequency  *string `json:"device-scan-frequency,omitempty"`
	Isolated             *string `json:"isolated,omitempty"`
	GPS                  *string `json:"gps,omitempty"`
	FogType              *string `json:"fog-type,omitempty"`
	DeveloperMode        *string `json:"developer-mode,omitempty"`
	LogsLevel            *string `json:"logs-level,omitempty"`
	TimeZone             *string `json:"time-zone,omitempty"`
}

// ConfigSetResponse represents the response for POST /v2/config
type ConfigSetResponse struct {
	Status   string            `json:"status"`
	ErrorMap map[string]string `json:"errorMap,omitempty"`
}

// HandleConfigGet handles GET /v2/config/get
func (h *ConfigHandler) HandleConfigGet(w http.ResponseWriter, r *http.Request) {
	logging.LogDebug(configHandlerModuleName, "Processing config get request")

	if r.Method != http.MethodPost {
		logging.LogError(configHandlerModuleName, "Request method not allowed", nil)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Read request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		logging.LogError(configHandlerModuleName, "Failed to read request body", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Parse JSON
	var req ConfigGetRequest
	if err := json.Unmarshal(body, &req); err != nil {
		logging.LogError(configHandlerModuleName, "Failed to parse JSON", err)
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Validate request
	if req.ID == "" {
		logging.LogError(configHandlerModuleName, "Missing ID in request", nil)
		http.Error(w, "Missing ID", http.StatusBadRequest)
		return
	}

	// Get container-specific config (matching Java GetConfigurationHandler)
	fieldAgent := fieldagent.GetInstance()
	containerConfig, exists := fieldAgent.GetContainerConfig(req.ID)

	var configStr string
	if exists && containerConfig != "" {
		// Return microservice-specific config as JSON string (matching Java)
		configStr = containerConfig
	} else {
		// Fallback to general config if no container-specific config found
		cfg := config.GetInstance()
		configMap := make(map[string]interface{})
		configMap["disk-limit"] = cfg.DiskLimit
		configMap["disk-directory"] = cfg.DiskDirectory
		configMap["memory-limit"] = cfg.MemoryLimit
		configMap["cpu-limit"] = cfg.CPULimit
		configMap["controller-url"] = cfg.ControllerURL
		configMap["docker-url"] = cfg.DockerURL
		configMap["network-adapter"] = cfg.NetworkInterface
		configMap["logs-limit"] = cfg.LogDiskLimit
		configMap["logs-directory"] = cfg.LogDiskDirectory
		configMap["logs-count"] = cfg.LogFileCount
		configMap["logs-level"] = cfg.LogLevel
		configMap["status-frequency"] = cfg.StatusFrequency
		configMap["changes-frequency"] = cfg.ChangeFrequency
		configMap["developer-mode"] = cfg.DevMode
		configMap["time-zone"] = cfg.TimeZone

		// Marshal to JSON string (matching Java behavior)
		configBytes, err := json.Marshal(configMap)
		if err != nil {
			logging.LogError(configHandlerModuleName, "Failed to marshal config", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		configStr = string(configBytes)
	}

	response := ConfigGetResponse{
		Status: "okay",
		Config: configStr, // Return as JSON string (matching Java)
	}

	jsonData, err := json.Marshal(response)
	if err != nil {
		logging.LogError(configHandlerModuleName, "Failed to marshal response", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if _, werr := w.Write(jsonData); werr != nil {
		logging.LogWarn(configHandlerModuleName, fmt.Sprintf("Failed to write response: %v", werr))
	}
	logging.LogDebug(configHandlerModuleName, "Finished processing config get request")
}

// HandleConfigSet handles POST /v2/config
func (h *ConfigHandler) HandleConfigSet(w http.ResponseWriter, r *http.Request) {
	logging.LogDebug(configHandlerModuleName, "Processing config set request")

	if r.Method != http.MethodPost {
		logging.LogError(configHandlerModuleName, "Request method not allowed", nil)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Validate content type
	contentType := r.Header.Get("Content-Type")
	if contentType != "application/json" {
		logging.LogError(configHandlerModuleName, "Invalid content type: "+contentType, nil)
		http.Error(w, "Content-Type must be application/json", http.StatusBadRequest)
		return
	}

	// Read request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		logging.LogError(configHandlerModuleName, "Failed to read request body", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Parse JSON
	var req ConfigSetRequest
	if err := json.Unmarshal(body, &req); err != nil {
		logging.LogError(configHandlerModuleName, "Failed to parse JSON", err)
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Build config map for setting
	configMap := make(map[string]interface{})

	// Map request fields to config parameter names
	if req.DiskLimit != nil {
		configMap["d"] = *req.DiskLimit
	}
	if req.DiskDirectory != nil {
		configMap["dl"] = *req.DiskDirectory
	}
	if req.MemoryLimit != nil {
		configMap["m"] = *req.MemoryLimit
	}
	if req.CPULimit != nil {
		configMap["p"] = *req.CPULimit
	}
	if req.ControllerURL != nil {
		configMap["a"] = *req.ControllerURL
	}
	if req.CertDirectory != nil {
		configMap["ac"] = *req.CertDirectory
	}
	if req.DockerURL != nil {
		configMap["c"] = *req.DockerURL
	}
	if req.ContainerEngine != nil {
		configMap["ce"] = *req.ContainerEngine
	}
	if req.NetworkAdapter != nil {
		configMap["n"] = *req.NetworkAdapter
	}
	if req.LogsLimit != nil {
		configMap["l"] = *req.LogsLimit
	}
	if req.LogsDirectory != nil {
		configMap["ld"] = *req.LogsDirectory
	}
	if req.LogsCount != nil {
		configMap["lc"] = *req.LogsCount
	}
	if req.StatusFrequency != nil {
		configMap["sf"] = *req.StatusFrequency
	}
	if req.ChangesFrequency != nil {
		configMap["cf"] = *req.ChangesFrequency
	}
	if req.DiagnosticsFrequency != nil {
		configMap["df"] = *req.DiagnosticsFrequency
	}
	if req.DeviceScanFrequency != nil {
		configMap["sd"] = *req.DeviceScanFrequency
	}
	if req.Isolated != nil {
		configMap["idc"] = *req.Isolated
	}
	if req.GPS != nil {
		configMap["gps"] = *req.GPS
	}
	if req.FogType != nil {
		configMap["ft"] = *req.FogType
	}
	if req.DeveloperMode != nil {
		configMap["dev"] = *req.DeveloperMode
	}
	if req.LogsLevel != nil {
		configMap["ll"] = *req.LogsLevel
	}
	if req.TimeZone != nil {
		configMap["tz"] = *req.TimeZone
	}

	// Call config.SetConfig() to set configuration values
	if len(configMap) == 0 {
		logging.LogError(configHandlerModuleName, "No valid config parameters provided", nil)
		http.Error(w, "Request not valid", http.StatusBadRequest)
		return
	}

	// Set config values and collect errors
	cfg := config.GetInstance()
	errorMapResult := cfg.SetConfig(configMap)

	// Notify modules if config was updated successfully (async to not block HTTP response)
	if len(errorMapResult) == 0 {
		go notifyModulesOfConfigChange()
	}

	// Convert error map keys from command line params to property names
	errorMessages := make(map[string]string)
	reverseMap := map[string]string{
		"d":    "disk-limit",
		"dl":   "disk-directory",
		"m":    "memory-limit",
		"p":    "cpu-limit",
		"a":    "controller-url",
		"ac":   "cert-directory",
		"c":    "docker-url",
		"n":    "network-adapter",
		"l":    "logs-limit",
		"ld":   "logs-directory",
		"lc":   "logs-count",
		"ll":   "logs-level",
		"sf":   "status-frequency",
		"cf":   "changes-frequency",
		"df":   "diagnostics-frequency",
		"sd":   "device-scan-frequency",
		"idc":  "isolated",
		"egf":  "edge-guard-frequency",
		"gps":  "gps",
		"gpsd": "gps-device",
		"gpsf": "gps-scan-frequency",
		"ft":   "fog-type",
		"sec":  "secure-mode",
		"pf":   "docker-pruning-frequency",
		"dt":   "available-disk-threshold",
		"uf":   "ready-to-upgrade-scan-frequency",
		"dev":  "developer-mode",
		"tz":   "time-zone",
	}

	for param, errMsg := range errorMapResult {
		if propName, exists := reverseMap[param]; exists {
			// Clean up error message (remove command line param references)
			cleanMsg := strings.ReplaceAll(errMsg, " -"+param+" ", " ")
			errorMessages[propName] = cleanMsg
		} else {
			errorMessages[param] = errMsg
		}
	}

	response := ConfigSetResponse{
		Status:   "okay",
		ErrorMap: errorMessages,
	}

	jsonData, err := json.Marshal(response)
	if err != nil {
		logging.LogError(configHandlerModuleName, "Failed to marshal response", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if _, werr := w.Write(jsonData); werr != nil {
		logging.LogWarn(configHandlerModuleName, fmt.Sprintf("Failed to write response: %v", werr))
	}
	logging.LogDebug(configHandlerModuleName, "Finished processing config set request")
}

// notifyModulesOfConfigChange notifies modules when configuration changes
func notifyModulesOfConfigChange() {
	// Trigger full config reload via Config callback (registered by Supervisor)
	if err := config.GetInstance().TriggerReloadCallback(); err != nil {
		logging.LogError(configHandlerModuleName, "Failed to reload agent configuration", err)
	}
}
