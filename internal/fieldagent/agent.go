package fieldagent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/eclipse-iofog/agent-go/internal/auth"
	"github.com/eclipse-iofog/agent-go/internal/config"
	"github.com/eclipse-iofog/agent-go/internal/diagnostics"
	"github.com/eclipse-iofog/agent-go/internal/hardware"
	"github.com/eclipse-iofog/agent-go/internal/models"
	"github.com/eclipse-iofog/agent-go/internal/processmanager"
	"github.com/eclipse-iofog/agent-go/internal/statusreporter"
	"github.com/eclipse-iofog/agent-go/internal/utils"
	"github.com/eclipse-iofog/agent-go/internal/utils/logging"
	"github.com/eclipse-iofog/agent-go/internal/volumemount"
)

const (
	moduleName    = "Field Agent"
	halHWInfoURL  = "http://localhost:54331/hal/hwc/lshw"
	halUSBInfoURL = "http://localhost:54331/hal/hwc/lsusb"
)

// getArchitectureCode converts architecture string to integer code
// Matches Java ArchitectureType.getCode():
// - 1 for INTEL_AMD (x86_64, amd64, etc.)
// - 2 for ARM (arm, arm64, aarch64, etc.)
// - 0 for UNDEFINED
func getArchitectureCode(arch string) int {
	archLower := strings.ToLower(arch)

	// Handle "auto" - detect from runtime
	if archLower == "auto" {
		goarch := runtime.GOARCH
		if goarch == "amd64" || goarch == "386" || goarch == "x86_64" {
			return 1 // INTEL_AMD
		}
		if strings.HasPrefix(goarch, "arm") || goarch == "arm64" || goarch == "aarch64" {
			return 2 // ARM
		}
		return 0 // UNDEFINED
	}

	// Handle explicit types
	if archLower == "intel_amd" || archLower == "x86_64" || archLower == "amd64" {
		return 1 // INTEL_AMD
	}
	if archLower == "arm" || archLower == "arm64" || archLower == "aarch64" {
		return 2 // ARM
	}

	return 0 // UNDEFINED
}

// FieldAgent handles all communication with the fog controller
type FieldAgent struct {
	config       *config.Config
	apiClient    *APIClient
	orchestrator *Orchestrator
	state        *State
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	mu           sync.RWMutex

	// Callbacks for other modules (will be set by supervisor)
	onMicroservicesUpdate func([]*models.Microservice) error
	onRegistriesUpdate    func([]*models.Registry) error
	onRoutesUpdate        func(map[string]*models.Route) error
	onConfigsUpdate       func(map[string]string) error
	processManager        *processmanager.ProcessManager

	// Exec session tracking
	activeExecSessions map[string]string                       // microserviceUUID -> execID
	execCallbacks      map[string]*ExecSessionCallback         // microserviceUUID -> callback
	activeWebSockets   map[string]*ExecSessionWebSocketHandler // microserviceUUID -> WebSocket handler (matching Java activeWebSockets)
	execSessionsMu     sync.RWMutex

	// Microservice management (for MicroserviceManagerInterface)
	latestMicroservices  []*models.Microservice
	currentMicroservices []*models.Microservice
	registries           []*models.Registry
	microservicesMu      sync.RWMutex

	// Edge resources
	edgeResources   []map[string]interface{}
	edgeResourcesMu sync.RWMutex

	// Container config map (matching Java ConfigurationMap.containerConfigMap)
	containerConfigMap map[string]string // microserviceUUID -> config JSON string
	containerConfigMu  sync.RWMutex

	// Provisioning lock (matching Java: provisioningLock)
	provisioningMu sync.Mutex
}

var (
	instance *FieldAgent
	once     sync.Once
)

// GetInstance returns the singleton FieldAgent instance
func GetInstance() *FieldAgent {
	once.Do(func() {
		instance = &FieldAgent{
			config:               config.GetInstance(),
			state:                NewState(),
			activeExecSessions:   make(map[string]string),
			execCallbacks:        make(map[string]*ExecSessionCallback),
			activeWebSockets:     make(map[string]*ExecSessionWebSocketHandler),
			latestMicroservices:  make([]*models.Microservice, 0),
			currentMicroservices: make([]*models.Microservice, 0),
			registries:           make([]*models.Registry, 0),
			containerConfigMap:   make(map[string]string),
		}
		// #region agent log
		logEntry := map[string]interface{}{
			"sessionId":    "debug-session",
			"runId":        "run1",
			"hypothesisId": "B",
			"location":     "agent.go:68",
			"message":      "GetInstance: FieldAgent created",
			"data":         map[string]interface{}{"ctx_is_nil": instance.ctx == nil},
			"timestamp":    time.Now().UnixMilli(),
		}
		if logBytes, err := json.Marshal(logEntry); err == nil {
			if f, err := os.OpenFile("/Users/emirhan/Documents/GitHub/Agent/.cursor/debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
				f.WriteString(string(logBytes) + "\n")
				f.Close()
			}
		}
		// #endregion
	})
	// #region agent log
	logEntry2 := map[string]interface{}{
		"sessionId":    "debug-session",
		"runId":        "run1",
		"hypothesisId": "B",
		"location":     "agent.go:80",
		"message":      "GetInstance: returning instance",
		"data":         map[string]interface{}{"ctx_is_nil": instance.ctx == nil},
		"timestamp":    time.Now().UnixMilli(),
	}
	if logBytes, err := json.Marshal(logEntry2); err == nil {
		if f, err := os.OpenFile("/Users/emirhan/Documents/GitHub/Agent/.cursor/debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
			f.WriteString(string(logBytes) + "\n")
			f.Close()
		}
	}
	// #endregion
	return instance
}

// Start starts the FieldAgent and all background workers
func (fa *FieldAgent) Start() error {
	logging.LogDebug(moduleName, "Starting Field Agent")

	// Initialize API client
	apiClient, err := NewAPIClient()
	if err != nil {
		logging.LogError(moduleName, "Failed to create API client", err)
		return err
	}
	fa.apiClient = apiClient
	logging.LogDebug(moduleName, "API client initialized")

	// Initialize Orchestrator
	fa.orchestrator = NewOrchestrator(apiClient)
	logging.LogDebug(moduleName, "Orchestrator initialized")

	// Set up Image Download Manager file uploader
	diagnostics.GetInstance().SetFileUploader(fa.orchestrator)

	// Create context for cancellation
	fa.ctx, fa.cancel = context.WithCancel(context.Background())
	// #region agent log
	logEntry := map[string]interface{}{
		"sessionId":    "debug-session",
		"runId":        "run1",
		"hypothesisId": "C",
		"location":     "agent.go:101",
		"message":      "Start: ctx initialized",
		"data":         map[string]interface{}{"ctx_is_nil": fa.ctx == nil},
		"timestamp":    time.Now().UnixMilli(),
	}
	if logBytes, err := json.Marshal(logEntry); err == nil {
		if f, err := os.OpenFile("/Users/emirhan/Documents/GitHub/Agent/.cursor/debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
			f.WriteString(string(logBytes) + "\n")
			f.Close()
		}
	}
	// #endregion

	// Initialize JWT Manager first if we have the private key (matching Java: lines 1960-1971)
	cfg := config.GetInstance()
	logging.LogDebug(moduleName, fmt.Sprintf("Checking provisioning status: UUID exists=%v, PrivateKey exists=%v",
		cfg.IOFogUUID != "", cfg.PrivateKey != ""))

	if cfg.IOFogUUID != "" && cfg.PrivateKey != "" {
		logging.LogDebug(moduleName, "Agent appears provisioned, initializing JWT Manager")
		// Try to generate JWT to verify private key is valid
		jwt, err := auth.GetJWTManager().GenerateJWT()
		if err != nil {
			logging.LogError(moduleName, "Failed to initialize JWT Manager", err)
			fa.state.SetControllerStatus(models.ControllerStatusNotProvisioned)
			fa.state.SetControllerVerified(false)
			// Update StatusReporter
			statusreporter.GetInstance().UpdateFieldAgentStatus(func(status *models.FieldAgentStatus) {
				status.ControllerStatus = models.ControllerStatusNotProvisioned
				status.ControllerVerified = false
			})
			logging.LogWarn(moduleName, "JWT initialization failed, setting status to NOT_PROVISIONED")
		} else {
			if jwt == "" {
				logging.LogError(moduleName, "JWT generation returned empty token", fmt.Errorf("JWT is empty"))
				fa.state.SetControllerStatus(models.ControllerStatusNotProvisioned)
				fa.state.SetControllerVerified(false)
				statusreporter.GetInstance().UpdateFieldAgentStatus(func(status *models.FieldAgentStatus) {
					status.ControllerStatus = models.ControllerStatusNotProvisioned
					status.ControllerVerified = false
				})
			} else {
				fa.state.SetControllerStatus(models.ControllerStatusOK)
				fa.state.SetControllerVerified(true)
				// Update StatusReporter
				statusreporter.GetInstance().UpdateFieldAgentStatus(func(status *models.FieldAgentStatus) {
					status.ControllerStatus = models.ControllerStatusOK
					status.ControllerVerified = true
				})
				logging.LogInfo(moduleName, "JWT Manager initialized successfully, setting status to OK")
			}
		}
	} else {
		logging.LogInfo(moduleName, "Agent not provisioned (missing UUID or privateKey), setting status to NOT_PROVISIONED")
		fa.state.SetControllerStatus(models.ControllerStatusNotProvisioned)
		fa.state.SetControllerVerified(false)
		// Update StatusReporter
		statusreporter.GetInstance().UpdateFieldAgentStatus(func(status *models.FieldAgentStatus) {
			status.ControllerStatus = models.ControllerStatusNotProvisioned
			status.ControllerVerified = false
		})
	}

	// Ping controller (matching Java: line 1980)
	logging.LogDebug(moduleName, "Pinging controller to verify connectivity")
	isConnected := fa.ping()
	logging.LogInfo(moduleName, fmt.Sprintf("Controller ping result: connected=%v", isConnected))

	currentStatus := fa.state.GetControllerStatus()
	logging.LogDebug(moduleName, fmt.Sprintf("Controller status after ping: %s", currentStatus))

	// Get fog config (matching Java: line 1981)
	logging.LogDebug(moduleName, "Fetching fog configuration from controller")
	if err := fa.getFogConfig(); err != nil {
		logging.LogWarn(moduleName, fmt.Sprintf("Failed to get fog config on startup: %v", err))
		// Don't fail startup for this
	} else {
		logging.LogDebug(moduleName, "Fog configuration fetched successfully")
	}

	// If provisioned, load initial data (matching Java: lines 1982-1991)
	if !fa.NotProvisioned() {
		logging.LogInfo(moduleName, "Agent is provisioned, loading initial data from controller")

		// Load registries
		logging.LogDebug(moduleName, "Loading registries")
		if err := fa.loadRegistries(!isConnected); err != nil {
			logging.LogWarn(moduleName, fmt.Sprintf("Failed to load registries on startup: %v", err))
		} else {
			logging.LogDebug(moduleName, "Registries loaded successfully")
		}

		// Load volume mounts (matching Java: loadVolumeMounts() - catches exceptions)
		logging.LogDebug(moduleName, "Start loading volume mounts")
		// Don't check error - Java version catches exceptions and continues
		func() {
			defer func() {
				if r := recover(); r != nil {
					logging.LogError(moduleName, fmt.Sprintf("Panic in loadVolumeMounts: %v", r), fmt.Errorf("%v", r))
				}
			}()
			fa.loadVolumeMounts() // Ignore error to match Java behavior
		}()
		logging.LogInfo(moduleName, "Volume mounts processing completed, proceeding to load microservices")

		// Load microservices
		logging.LogDebug(moduleName, "Start Loading microservices...")
		microservices, err := fa.loadMicroservices(!isConnected)
		if err != nil {
			logging.LogWarn(moduleName, fmt.Sprintf("Failed to load microservices on startup: %v", err))
		} else {
			logging.LogInfo(moduleName, fmt.Sprintf("Loaded %d microservices from controller/cache", len(microservices)))
			// Process microservice config
			logging.LogDebug(moduleName, "Start process microservice configuration")
			if err := fa.processMicroserviceConfig(microservices); err != nil {
				logging.LogWarn(moduleName, fmt.Sprintf("Failed to process microservice config on startup: %v", err))
			} else {
				logging.LogInfo(moduleName, "Microservice config processed successfully")
			}
			logging.LogDebug(moduleName, "Finished process microservice configuration")

			// Process routes
			logging.LogDebug(moduleName, "Start processing routes")
			if err := fa.processRoutes(microservices); err != nil {
				logging.LogWarn(moduleName, fmt.Sprintf("Failed to process routes on startup: %v", err))
			} else {
				logging.LogInfo(moduleName, "Routes processed successfully")
			}
			logging.LogDebug(moduleName, "Finished process routes")
		}

		// Notify ProcessManager to immediately update (matching Java: line 1990)
		// This ensures containers are processed during initialization without waiting
		if fa.processManager != nil {
			logging.LogDebug(moduleName, "Notifying ProcessManager to update")
			fa.processManager.Update()
		} else {
			logging.LogWarn(moduleName, "ProcessManager not set, skipping update notification")
		}

		// Load edge resources
		logging.LogDebug(moduleName, "Loading edge resources")
		if err := fa.loadEdgeResources(!isConnected); err != nil {
			logging.LogWarn(moduleName, fmt.Sprintf("Failed to load edge resources on startup: %v", err))
		} else {
			logging.LogDebug(moduleName, "Edge resources loaded successfully")
		}

		logging.LogInfo(moduleName, "Initial data loading completed")
	} else {
		logging.LogInfo(moduleName, "Agent not provisioned, skipping initial data load")
	}

	// Start background workers (matching Java: lines 1995-1998)
	logging.LogDebug(moduleName, "Starting background workers")
	fa.wg.Add(4)
	go fa.pingControllerWorker()
	go fa.getChangesWorker()
	go fa.postStatusWorker()
	go fa.postDiagnosticsWorker()

	logging.LogInfo(moduleName, "Field Agent started successfully")
	return nil
}

// Stop stops the FieldAgent and all background workers
func (fa *FieldAgent) Stop() error {
	logging.LogDebug(moduleName, "Stopping Field Agent")

	if fa.cancel != nil {
		fa.cancel()
	}

	// Wait for all workers to finish
	fa.wg.Wait()

	logging.LogDebug(moduleName, "Field Agent stopped")
	return nil
}

// GetName returns the module name
func (fa *FieldAgent) GetName() string {
	return moduleName
}

// GetModuleIndex returns the module index
func (fa *FieldAgent) GetModuleIndex() int {
	return utils.FieldAgent
}

// SetOnMicroservicesUpdate sets the callback for microservices updates
func (fa *FieldAgent) SetOnMicroservicesUpdate(callback func([]*models.Microservice) error) {
	fa.mu.Lock()
	defer fa.mu.Unlock()
	fa.onMicroservicesUpdate = callback
}

// SetOnRegistriesUpdate sets the callback for registries updates
func (fa *FieldAgent) SetOnRegistriesUpdate(callback func([]*models.Registry) error) {
	fa.mu.Lock()
	defer fa.mu.Unlock()
	fa.onRegistriesUpdate = callback
}

// SetOnRoutesUpdate sets the callback for routes updates
func (fa *FieldAgent) SetOnRoutesUpdate(callback func(map[string]*models.Route) error) {
	fa.mu.Lock()
	defer fa.mu.Unlock()
	fa.onRoutesUpdate = callback
}

// SetOnConfigsUpdate sets the callback for configs updates
func (fa *FieldAgent) SetOnConfigsUpdate(callback func(map[string]string) error) {
	fa.mu.Lock()
	defer fa.mu.Unlock()
	fa.onConfigsUpdate = callback
}

// GetContainerConfig returns the container config string for a microservice UUID
func (fa *FieldAgent) GetContainerConfig(microserviceUUID string) (string, bool) {
	fa.containerConfigMu.RLock()
	defer fa.containerConfigMu.RUnlock()
	config, exists := fa.containerConfigMap[microserviceUUID]
	return config, exists
}

// SetProcessManager sets the ProcessManager reference
func (fa *FieldAgent) SetProcessManager(pm *processmanager.ProcessManager) {
	fa.mu.Lock()
	defer fa.mu.Unlock()
	fa.processManager = pm
}

// NotProvisioned checks if the agent is not provisioned
func (fa *FieldAgent) NotProvisioned() bool {
	logging.LogDebug(moduleName, "Started checking provisioned")
	status := fa.state.GetControllerStatus()
	notProvisioned := status == models.ControllerStatusNotProvisioned
	if notProvisioned {
		logging.LogWarn(moduleName, "Not provisioned")
	}
	logging.LogDebug(moduleName, fmt.Sprintf("Finished checking provisioned: %v", !notProvisioned))
	return notProvisioned
}

// IsControllerConnected checks if the controller is connected
func (fa *FieldAgent) IsControllerConnected(fromFile bool) bool {
	logging.LogDebug(moduleName, "check is Controller Connected")

	isConnected := false
	status := fa.state.GetControllerStatus()

	if status != models.ControllerStatusOK && !fromFile {
		if !fa.ping() {
			fa.handleBadControllerStatus()
		} else {
			isConnected = true
		}
	} else {
		isConnected = true
	}

	logging.LogDebug(moduleName, fmt.Sprintf("checked is Controller Connected: %v", isConnected))
	return isConnected
}

// handleBadControllerStatus handles bad controller status
func (fa *FieldAgent) handleBadControllerStatus() {
	logging.LogDebug(moduleName, "Start handle Bad Controller Status")
	errMsg := "Connection to controller has broken"
	if fa.state.IsControllerVerified() {
		logging.LogWarn(moduleName, errMsg)
	} else {
		fa.verificationFailed(fmt.Errorf("%s", errMsg))
	}
	logging.LogDebug(moduleName, "Finished handling Bad Controller Status")
}

// verificationFailed handles controller verification failure
func (fa *FieldAgent) verificationFailed(err error) {
	logging.LogDebug(moduleName, "Start verification Failed of controller")

	fa.state.SetConnected(false)

	if !fa.NotProvisioned() {
		var controllerStatus models.ControllerStatus
		// Check if it's a certificate error
		if isCertificateError(err) {
			controllerStatus = models.ControllerStatusBrokenCertificate
		} else {
			controllerStatus = models.ControllerStatusNotConnected
		}
		fa.state.SetControllerStatus(controllerStatus)
		// Update StatusReporter (matching Java behavior)
		statusreporter.GetInstance().UpdateFieldAgentStatus(func(status *models.FieldAgentStatus) {
			status.ControllerStatus = controllerStatus
			status.ControllerVerified = false
		})
		logging.LogWarn(moduleName, fmt.Sprintf("controller verification failed: %s", controllerStatus))
	}
	fa.state.SetControllerVerified(false)
	logging.LogDebug(moduleName, "Finished verification Failed of Controller")
}

// isCertificateError checks if an error is a certificate-related error
func isCertificateError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return contains(errStr, "certificate") || contains(errStr, "tls") || contains(errStr, "ssl")
}

// contains checks if a string contains a substring (case-insensitive)
func contains(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}

// ping pings the controller to check connectivity
func (fa *FieldAgent) ping() bool {
	logging.LogDebug(moduleName, "Started Ping")

	if fa.NotProvisioned() {
		logging.LogDebug(moduleName, "Agent not provisioned, skipping ping")
		logging.LogInfo(moduleName, "Finished Ping: false (not provisioned)")
		return false
	}

	ctx, cancel := context.WithTimeout(fa.ctx, 30*time.Second)
	defer cancel()

	logging.LogDebug(moduleName, "Calling API client ping")
	ok, err := fa.apiClient.Ping(ctx)
	if err != nil {
		logging.LogError(moduleName, fmt.Sprintf("Error pinging controller: %v", err), err)
		fa.verificationFailed(err)
		logging.LogDebug(moduleName, "Finished Ping: false (error)")
		return false
	}

	if ok {
		fa.state.SetControllerStatus(models.ControllerStatusOK)
		fa.state.SetControllerVerified(true)
		// Update StatusReporter (matching Java: StatusReporter.setFieldAgentStatus().setControllerStatus(OK))
		statusreporter.GetInstance().UpdateFieldAgentStatus(func(status *models.FieldAgentStatus) {
			status.ControllerStatus = models.ControllerStatusOK
			status.ControllerVerified = true
		})
		logging.LogInfo(moduleName, "Controller ping successful, status set to OK")
		logging.LogDebug(moduleName, fmt.Sprintf("Updated StatusReporter: ControllerStatus=%s, ControllerVerified=%v",
			models.ControllerStatusOK, true))

		// Verify the update was applied
		verifyStatus := statusreporter.GetInstance().GetFieldAgentStatus()
		logging.LogDebug(moduleName, fmt.Sprintf("Verified StatusReporter state: ControllerStatus=%s, ControllerVerified=%v",
			verifyStatus.ControllerStatus, verifyStatus.ControllerVerified))

		logging.LogDebug(moduleName, "Finished Ping: true")
		return true
	}

	logging.LogWarn(moduleName, "Controller ping returned false (no status in response)")
	logging.LogDebug(moduleName, "Finished Ping: false")
	return false
}

// Provision provisions the agent with the controller
func (fa *FieldAgent) Provision(key string) error {
	// #region agent log
	logEntry := map[string]interface{}{
		"sessionId":    "debug-session",
		"runId":        "run1",
		"hypothesisId": "A",
		"location":     "agent.go:279",
		"message":      "Provision entry",
		"data":         map[string]interface{}{"keyLength": len(key), "fa_ctx_is_nil": fa.ctx == nil},
		"timestamp":    time.Now().UnixMilli(),
	}
	if logBytes, err := json.Marshal(logEntry); err == nil {
		if f, err := os.OpenFile("/Users/emirhan/Documents/GitHub/Agent/.cursor/debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
			f.WriteString(string(logBytes) + "\n")
			f.Close()
		}
	}
	// #endregion
	logging.LogDebug(moduleName, "Start provisioning")

	// #region agent log
	logEntry2 := map[string]interface{}{
		"sessionId":    "debug-session",
		"runId":        "run1",
		"hypothesisId": "A",
		"location":     "agent.go:282",
		"message":      "Before context.WithTimeout",
		"data":         map[string]interface{}{"fa_ctx_is_nil": fa.ctx == nil, "fa_ctx_type": fmt.Sprintf("%T", fa.ctx)},
		"timestamp":    time.Now().UnixMilli(),
	}
	if logBytes, err := json.Marshal(logEntry2); err == nil {
		if f, err := os.OpenFile("/Users/emirhan/Documents/GitHub/Agent/.cursor/debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
			f.WriteString(string(logBytes) + "\n")
			f.Close()
		}
	}
	// #endregion

	// Use fa.ctx if available (daemon mode), otherwise use context.Background() (CLI mode)
	parentCtx := fa.ctx
	if parentCtx == nil {
		parentCtx = context.Background()
	}
	ctx, cancel := context.WithTimeout(parentCtx, 30*time.Second)
	defer cancel()

	// #region agent log
	logEntry3 := map[string]interface{}{
		"sessionId":    "debug-session",
		"runId":        "post-fix",
		"hypothesisId": "A",
		"location":     "agent.go:295",
		"message":      "After context.WithTimeout (post-fix)",
		"data":         map[string]interface{}{"parent_ctx_was_nil": fa.ctx == nil, "using_background": fa.ctx == nil},
		"timestamp":    time.Now().UnixMilli(),
	}
	if logBytes, err := json.Marshal(logEntry3); err == nil {
		if f, err := os.OpenFile("/Users/emirhan/Documents/GitHub/Agent/.cursor/debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
			f.WriteString(string(logBytes) + "\n")
			f.Close()
		}
	}
	// #endregion

	// #region agent log
	logEntry4 := map[string]interface{}{
		"sessionId":    "debug-session",
		"runId":        "run2",
		"hypothesisId": "F",
		"location":     "agent.go:396",
		"message":      "Before apiClient.Request",
		"data":         map[string]interface{}{"apiClient_is_nil": fa.apiClient == nil},
		"timestamp":    time.Now().UnixMilli(),
	}
	if logBytes, err := json.Marshal(logEntry4); err == nil {
		if f, err := os.OpenFile("/Users/emirhan/Documents/GitHub/Agent/.cursor/debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
			f.WriteString(string(logBytes) + "\n")
			f.Close()
		}
	}
	// #endregion

	// Initialize apiClient if nil (CLI mode - Start() hasn't been called)
	if fa.apiClient == nil {
		// #region agent log
		logEntry5 := map[string]interface{}{
			"sessionId":    "debug-session",
			"runId":        "run2",
			"hypothesisId": "F",
			"location":     "agent.go:410",
			"message":      "Initializing apiClient lazily",
			"data":         map[string]interface{}{},
			"timestamp":    time.Now().UnixMilli(),
		}
		if logBytes, err := json.Marshal(logEntry5); err == nil {
			if f, err := os.OpenFile("/Users/emirhan/Documents/GitHub/Agent/.cursor/debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
				f.WriteString(string(logBytes) + "\n")
				f.Close()
			}
		}
		// #endregion
		apiClient, err := NewAPIClient()
		if err != nil {
			return fmt.Errorf("failed to initialize API client: %w", err)
		}
		fa.apiClient = apiClient
	}

	// Get architecture code from config
	// Java sends integer code: 1 for INTEL_AMD, 2 for ARM, 0 for UNDEFINED
	cfg := config.GetInstance()
	archCode := getArchitectureCode(cfg.Arch)
	body := map[string]interface{}{
		"key":  key,
		"type": archCode,
	}

	// #region agent log
	logEntry6 := map[string]interface{}{
		"sessionId":    "debug-session",
		"runId":        "run4",
		"hypothesisId": "H",
		"location":     "agent.go:443",
		"message":      "Provision: request body",
		"data":         map[string]interface{}{"body": body, "arch": cfg.Arch},
		"timestamp":    time.Now().UnixMilli(),
	}
	if logBytes, err := json.Marshal(logEntry6); err == nil {
		if f, err := os.OpenFile("/Users/emirhan/Documents/GitHub/Agent/.cursor/debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
			f.WriteString(string(logBytes) + "\n")
			f.Close()
		}
	}
	// #endregion

	result, err := fa.apiClient.Request(ctx, "provision", POST, nil, body)
	if err != nil {
		return fmt.Errorf("provisioning failed: %w", err)
	}

	// #region agent log
	logEntry7 := map[string]interface{}{
		"sessionId":    "debug-session",
		"runId":        "run5",
		"hypothesisId": "I",
		"location":     "agent.go:499",
		"message":      "Provision: result received",
		"data": map[string]interface{}{
			"has_uuid":        result["uuid"] != nil,
			"has_privateKey":  result["privateKey"] != nil,
			"has_namespace":   result["namespace"] != nil,
			"uuid_type":       fmt.Sprintf("%T", result["uuid"]),
			"privateKey_type": fmt.Sprintf("%T", result["privateKey"]),
			"namespace_type":  fmt.Sprintf("%T", result["namespace"]),
		},
		"timestamp": time.Now().UnixMilli(),
	}
	if logBytes, err := json.Marshal(logEntry7); err == nil {
		if f, err := os.OpenFile("/Users/emirhan/Documents/GitHub/Agent/.cursor/debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
			f.WriteString(string(logBytes) + "\n")
			f.Close()
		}
	}
	// #endregion

	// Extract UUID, private key, and namespace from result and save to config
	// Matching Java: Configuration.setIofogUuid(), setPrivateKey(), setNamespace(), saveConfigUpdates()
	updated := false

	if uuid, ok := result["uuid"].(string); ok && uuid != "" {
		cfg.IOFogUUID = uuid
		if err = cfg.SetProperty("iofogUuid", uuid); err != nil {
			logging.LogWarn(moduleName, fmt.Sprintf("Failed to set iofogUuid in YAML config: %v", err))
		}
		updated = true
		// #region agent log
		logEntry8 := map[string]interface{}{
			"sessionId":    "debug-session",
			"runId":        "run5",
			"hypothesisId": "I",
			"location":     "agent.go:520",
			"message":      "Provision: UUID set",
			"data":         map[string]interface{}{"uuid": uuid},
			"timestamp":    time.Now().UnixMilli(),
		}
		if logBytes, err := json.Marshal(logEntry8); err == nil {
			if f, err := os.OpenFile("/Users/emirhan/Documents/GitHub/Agent/.cursor/debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
				f.WriteString(string(logBytes) + "\n")
				f.Close()
			}
		}
		// #endregion
	}
	if privateKey, ok := result["privateKey"].(string); ok && privateKey != "" {
		cfg.PrivateKey = privateKey
		if err = cfg.SetProperty("privateKey", privateKey); err != nil {
			logging.LogWarn(moduleName, fmt.Sprintf("Failed to set privateKey in YAML config: %v", err))
		}
		updated = true
		// #region agent log
		logEntry9 := map[string]interface{}{
			"sessionId":    "debug-session",
			"runId":        "run5",
			"hypothesisId": "I",
			"location":     "agent.go:538",
			"message":      "Provision: privateKey set",
			"data":         map[string]interface{}{"privateKey_length": len(privateKey)},
			"timestamp":    time.Now().UnixMilli(),
		}
		if logBytes, err := json.Marshal(logEntry9); err == nil {
			if f, err := os.OpenFile("/Users/emirhan/Documents/GitHub/Agent/.cursor/debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
				f.WriteString(string(logBytes) + "\n")
				f.Close()
			}
		}
		// #endregion
	}
	if namespace, ok := result["namespace"].(string); ok && namespace != "" {
		cfg.Namespace = namespace
		if err = cfg.SetProperty("namespace", namespace); err != nil {
			logging.LogWarn(moduleName, fmt.Sprintf("Failed to set namespace in YAML config: %v", err))
		}
		updated = true
		// #region agent log
		logEntry10 := map[string]interface{}{
			"sessionId":    "debug-session",
			"runId":        "run5",
			"hypothesisId": "I",
			"location":     "agent.go:556",
			"message":      "Provision: namespace set",
			"data":         map[string]interface{}{"namespace": namespace},
			"timestamp":    time.Now().UnixMilli(),
		}
		if logBytes, err := json.Marshal(logEntry10); err == nil {
			if f, err := os.OpenFile("/Users/emirhan/Documents/GitHub/Agent/.cursor/debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
				f.WriteString(string(logBytes) + "\n")
				f.Close()
			}
		}
		// #endregion
	}

	// Save config to disk (matching Java: Configuration.saveConfigUpdates())
	if updated {
		configPath := utils.ConfigYAMLPath
		if err := config.SaveConfig(configPath); err != nil {
			logging.LogError(moduleName, "Failed to save config updates after provisioning", err)
			return fmt.Errorf("provisioning succeeded but failed to save config: %w", err)
		}
		// #region agent log
		logEntry11 := map[string]interface{}{
			"sessionId":    "debug-session",
			"runId":        "run5",
			"hypothesisId": "I",
			"location":     "agent.go:575",
			"message":      "Provision: config saved to disk",
			"data":         map[string]interface{}{"configPath": configPath},
			"timestamp":    time.Now().UnixMilli(),
		}
		if logBytes, err := json.Marshal(logEntry11); err == nil {
			if f, err := os.OpenFile("/Users/emirhan/Documents/GitHub/Agent/.cursor/debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
				f.WriteString(string(logBytes) + "\n")
				f.Close()
			}
		}
		// #endregion
	}

	// Reset JWT manager to use new credentials
	// IMPORTANT: Reset AFTER config is saved so JWT manager can reload from updated config
	auth.GetJWTManager().Reset()

	// #region agent log
	logEntry12a := map[string]interface{}{
		"sessionId":    "debug-session",
		"runId":        "run6",
		"hypothesisId": "M",
		"location":     "agent.go:633",
		"message":      "Provision: JWT manager reset, checking config",
		"data": map[string]interface{}{
			"uuid":           cfg.IOFogUUID,
			"privateKey_len": len(cfg.PrivateKey),
			"has_uuid":       cfg.IOFogUUID != "",
			"has_privateKey": cfg.PrivateKey != "",
		},
		"timestamp": time.Now().UnixMilli(),
	}
	if logBytes, err := json.Marshal(logEntry12a); err == nil {
		if f, err := os.OpenFile("/Users/emirhan/Documents/GitHub/Agent/.cursor/debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
			f.WriteString(string(logBytes) + "\n")
			f.Close()
		}
	}
	// #endregion

	// Recreate API client with new credentials (matching Java: orchestrator.update() after provisioning)
	// This is critical because the API client was created before provisioning (without UUID/privateKey)
	fa.mu.Lock()
	apiClient, err := NewAPIClient()
	if err != nil {
		fa.mu.Unlock()
		logging.LogError(moduleName, "Failed to recreate API client after provisioning", err)
		return fmt.Errorf("provisioning succeeded but failed to recreate API client: %w", err)
	}
	fa.apiClient = apiClient

	// Update orchestrator with new API client
	if fa.orchestrator != nil {
		fa.orchestrator = NewOrchestrator(apiClient)
	}
	fa.mu.Unlock()

	// #region agent log
	logEntry12b := map[string]interface{}{
		"sessionId":    "debug-session",
		"runId":        "run6",
		"hypothesisId": "M",
		"location":     "agent.go:650",
		"message":      "Provision: API client recreated, testing JWT generation",
		"data":         map[string]interface{}{"apiClient_is_nil": apiClient == nil},
		"timestamp":    time.Now().UnixMilli(),
	}
	if logBytes, err := json.Marshal(logEntry12b); err == nil {
		if f, err := os.OpenFile("/Users/emirhan/Documents/GitHub/Agent/.cursor/debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
			f.WriteString(string(logBytes) + "\n")
			f.Close()
		}
	}
	// #endregion

	// Test JWT generation to ensure it works
	testToken, testErr := auth.GetJWTManager().GenerateJWT()
	if testErr != nil {
		// #region agent log
		logEntry12c := map[string]interface{}{
			"sessionId":    "debug-session",
			"runId":        "run6",
			"hypothesisId": "M",
			"location":     "agent.go:665",
			"message":      "Provision: JWT generation test failed",
			"data":         map[string]interface{}{"error": testErr.Error()},
			"timestamp":    time.Now().UnixMilli(),
		}
		if logBytes, err := json.Marshal(logEntry12c); err == nil {
			if f, err := os.OpenFile("/Users/emirhan/Documents/GitHub/Agent/.cursor/debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
				f.WriteString(string(logBytes) + "\n")
				f.Close()
			}
		}
		// #endregion
		logging.LogWarn(moduleName, fmt.Sprintf("JWT generation test failed after provisioning: %v", testErr))
	} else {
		// #region agent log
		logEntry12d := map[string]interface{}{
			"sessionId":    "debug-session",
			"runId":        "run6",
			"hypothesisId": "M",
			"location":     "agent.go:680",
			"message":      "Provision: JWT generation test succeeded",
			"data":         map[string]interface{}{"token_length": len(testToken)},
			"timestamp":    time.Now().UnixMilli(),
		}
		if logBytes, err := json.Marshal(logEntry12d); err == nil {
			if f, err := os.OpenFile("/Users/emirhan/Documents/GitHub/Agent/.cursor/debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
				f.WriteString(string(logBytes) + "\n")
				f.Close()
			}
		}
		// #endregion
	}

	// #region agent log
	logEntry12 := map[string]interface{}{
		"sessionId":    "debug-session",
		"runId":        "run6",
		"hypothesisId": "J",
		"location":     "agent.go:633",
		"message":      "Provision: API client recreated",
		"data":         map[string]interface{}{"apiClient_is_nil": apiClient == nil},
		"timestamp":    time.Now().UnixMilli(),
	}
	if logBytes, err := json.Marshal(logEntry12); err == nil {
		if f, err := os.OpenFile("/Users/emirhan/Documents/GitHub/Agent/.cursor/debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
			f.WriteString(string(logBytes) + "\n")
			f.Close()
		}
	}
	// #endregion

	// Set status to OK since provisioning succeeded (matching Java: StatusReporter.setFieldAgentStatus().setControllerStatus(OK))
	fa.state.SetControllerStatus(models.ControllerStatusOK)
	fa.state.SetControllerVerified(true)

	// If daemon is running (ctx is set), update the API client and post config
	// This ensures the daemon's FieldAgent uses the new credentials
	if fa.ctx != nil {
		// Update FieldAgent to reload config and recreate API client with new credentials
		// This is critical: the daemon's FieldAgent was created before provisioning
		if err := fa.Update(); err != nil {
			logging.LogWarn(moduleName, fmt.Sprintf("Failed to update FieldAgent after provisioning: %v", err))
		} else {
			// #region agent log
			logEntry13 := map[string]interface{}{
				"sessionId":    "debug-session",
				"runId":        "run6",
				"hypothesisId": "K",
				"location":     "agent.go:675",
				"message":      "Provision: FieldAgent updated (daemon mode)",
				"data":         map[string]interface{}{},
				"timestamp":    time.Now().UnixMilli(),
			}
			if logBytes, err := json.Marshal(logEntry13); err == nil {
				if f, err := os.OpenFile("/Users/emirhan/Documents/GitHub/Agent/.cursor/debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
					f.WriteString(string(logBytes) + "\n")
					f.Close()
				}
			}
			// #endregion
		}

		// Post fog config to controller (matching Java: postFogConfig() after provisioning)
		// This sends the agent configuration to the controller and establishes the connection
		if err := fa.postFogConfig(); err != nil {
			logging.LogWarn(moduleName, fmt.Sprintf("Failed to post fog config after provisioning (non-critical): %v", err))
			// Don't fail provisioning for this - matching Java behavior
		} else {
			// #region agent log
			logEntry14 := map[string]interface{}{
				"sessionId":    "debug-session",
				"runId":        "run6",
				"hypothesisId": "K",
				"location":     "agent.go:695",
				"message":      "Provision: postFogConfig succeeded",
				"data":         map[string]interface{}{},
				"timestamp":    time.Now().UnixMilli(),
			}
			if logBytes, err := json.Marshal(logEntry14); err == nil {
				if f, err := os.OpenFile("/Users/emirhan/Documents/GitHub/Agent/.cursor/debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
					f.WriteString(string(logBytes) + "\n")
					f.Close()
				}
			}
			// #endregion
		}
	} else {
		// #region agent log
		logEntry15 := map[string]interface{}{
			"sessionId":    "debug-session",
			"runId":        "run6",
			"hypothesisId": "K",
			"location":     "agent.go:712",
			"message":      "Provision: skipping update/postFogConfig (CLI mode, ctx is nil)",
			"data":         map[string]interface{}{},
			"timestamp":    time.Now().UnixMilli(),
		}
		if logBytes, err := json.Marshal(logEntry15); err == nil {
			if f, err := os.OpenFile("/Users/emirhan/Documents/GitHub/Agent/.cursor/debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
				f.WriteString(string(logBytes) + "\n")
				f.Close()
			}
		}
		// #endregion
	}

	logging.LogDebug(moduleName, "Finished provisioning")
	return nil
}

// getDeprovisionBody builds the deprovision request body with microservice UUIDs
// (matching Java: getDeprovisionBody())
func (fa *FieldAgent) getDeprovisionBody() map[string]interface{} {
	// Get all microservice UUIDs from latest and current (using a set to avoid duplicates)
	uuidSet := make(map[string]bool)
	
	latest := fa.GetLatestMicroservices()
	current := fa.GetCurrentMicroservices()
	
	for _, ms := range latest {
		if ms.MicroserviceUUID != "" {
			uuidSet[ms.MicroserviceUUID] = true
		}
	}
	for _, ms := range current {
		if ms.MicroserviceUUID != "" {
			uuidSet[ms.MicroserviceUUID] = true
		}
	}
	
	// Convert set to array
	uuids := make([]string, 0, len(uuidSet))
	for uuid := range uuidSet {
		uuids = append(uuids, uuid)
	}
	
	return map[string]interface{}{
		"microserviceUuids": uuids,
	}
}

// Deprovision deprovisions the agent (matching Java: deProvision(boolean isTokenExpired))
// clearCredentials=true matches Java's isTokenExpired=true (skip controller request)
func (fa *FieldAgent) Deprovision(clearCredentials bool) error {
	logging.LogInfo(moduleName, "Start Deprovisioning")

	// Acquire provisioning lock (matching Java: provisioningLock.tryLock())
	if !fa.provisioningMu.TryLock() {
		msg := "Provisioning in progress"
		logging.LogInfo(moduleName, msg)
		return fmt.Errorf(msg)
	}
	defer fa.provisioningMu.Unlock()

	// Check if already not provisioned (matching Java: notProvisioned())
	if fa.NotProvisioned() {
		logging.LogInfo(moduleName, "Finished Deprovisioning : Failure - not provisioned")
		return fmt.Errorf("\nFailure - not provisioned")
	}

	// Store configuration values before clearing them (matching Java)
	iofogUuid := fa.config.IOFogUUID
	// Note: privateKey and namespace are stored but not used in Go (matching Java behavior)

	// Delete hardware signature file (.jwt) BEFORE clearing UUID
	// The file path is based on IOFogUUID, so we need to delete it while UUID still exists
	func() {
		defer func() {
			if r := recover(); r != nil {
				logging.LogWarn(moduleName, fmt.Sprintf("Error deleting hardware signature file: %v", r))
			}
		}()
		if err := hardware.DeleteHardwareSignature(); err != nil {
			logging.LogDebug(moduleName, fmt.Sprintf("Hardware signature file deletion attempted (may not exist): %v", err))
		} else {
			logging.LogDebug(moduleName, "Hardware signature file deleted on deprovision")
		}
	}()

	// Attempt deprovision request if not token expired (matching Java: !isTokenExpired)
	deprovisionRequestSuccessful := false
	if !clearCredentials {
		logging.LogDebug(moduleName, "Attempting deprovision request to controller")
		ctx := context.Background()
		_, err := fa.orchestrator.Request(ctx, "deprovision", POST, nil, fa.getDeprovisionBody())
		if err != nil {
			logging.LogError(moduleName, "Unable to make deprovision request", err)
		} else {
			logging.LogInfo(moduleName, "Deprovision request completed successfully")
			deprovisionRequestSuccessful = true
		}
	} else {
		logging.LogInfo(moduleName, "Skipping deprovision request due to expired token")
	}

	// Update status to NOT_PROVISIONED (matching Java)
	statusreporter.GetInstance().UpdateFieldAgentStatus(func(status *models.FieldAgentStatus) {
		status.ControllerStatus = models.ControllerStatusNotProvisioned
	})

	// Clear configuration AFTER the deprovision request attempt (matching Java)
	configUpdated := true
	func() {
		defer func() {
			if r := recover(); r != nil {
				configUpdated = false
				logging.LogError(moduleName, "Error saving config updates", fmt.Errorf("%v", r))
			}
		}()
		
		fa.config.IOFogUUID = ""
		fa.config.PrivateKey = ""
		fa.config.Namespace = "default"
		
		// Update YAML config properties before saving (matching Java: Configuration.saveConfigUpdates())
		// The SaveConfig() function uses GetYamlConfig() which doesn't reflect in-memory struct changes
		// We need to update the YAML config's Properties map directly
		cfg := config.GetInstance()
		yamlConfig := cfg.GetYamlConfig()
		if yamlConfig != nil {
			profile := yamlConfig.GetProfile(cfg.GetCurrentProfile().FullValue())
			if profile != nil {
				profile.SetProperty("iofogUuid", "")
				profile.SetProperty("privateKey", "")
				profile.SetProperty("namespace", "default")
				logging.LogDebug(moduleName, "Updated YAML config properties: iofogUuid, privateKey, namespace")
			} else {
				logging.LogWarn(moduleName, "Profile not found in YAML config, cannot update properties")
			}
		} else {
			logging.LogWarn(moduleName, "YAML config not loaded, cannot update properties")
		}
		
		// Save config updates (matching Java: Configuration.saveConfigUpdates())
		// Use SaveConfig with the config path
		configPath := utils.ConfigYAMLPath
		if err := config.SaveConfig(configPath); err != nil {
			logging.LogError(moduleName, "Error saving config updates", err)
			configUpdated = false
		} else {
			logging.LogDebug(moduleName, "Configuration cleared successfully")
		}
		
		// Reset JWT Manager (matching Java: JwtManager.reset())
		func() {
			defer func() {
				if r := recover(); r != nil {
					logging.LogWarn(moduleName, fmt.Sprintf("Failed to reset JWT Manager: %v", r))
					// Don't fail deprovisioning for JWT reset failure
				}
			}()
			auth.GetJWTManager().Reset()
			logging.LogDebug(moduleName, "JWT Manager reset completed")
		}()
	}()
	
	if configUpdated {
		// Update config backup file (matching Java: Configuration.updateConfigBackUpFile())
		// Note: This might not be implemented in Go yet, but we'll log it
		logging.LogDebug(moduleName, "Config backup file update requested")
	}

	// Clear microservice manager (matching Java: microserviceManager.clear())
	fa.Clear()

	// Stop running microservices (matching Java: ProcessManager.getInstance().stopRunningMicroservices(false, iofogUuid))
	if fa.processManager != nil {
		func() {
			defer func() {
				if r := recover(); r != nil {
					logging.LogError(moduleName, "Error stopping running microservices", fmt.Errorf("%v", r))
				}
			}()
			if err := fa.processManager.StopRunningMicroservices(iofogUuid); err != nil {
				logging.LogError(moduleName, "Error stopping running microservices", err)
			}
		}()
	}

	// Clear volume mounts (matching Java: volumeMountManager.clear())
	func() {
		defer func() {
			if r := recover(); r != nil {
				logging.LogError(moduleName, "Error clearing volume mounts", fmt.Errorf("%v", r))
			}
		}()
		if err := volumemount.GetInstance().Clear(); err != nil {
			logging.LogError(moduleName, "Error clearing volume mounts", err)
		}
	}()

	// Delete JSON cache files (matching Java: deleteNode cleanup)
	// These files are stored in config directory and should be cleaned on deprovision
	func() {
		defer func() {
			if r := recover(); r != nil {
				logging.LogWarn(moduleName, fmt.Sprintf("Error deleting JSON cache files: %v", r))
			}
		}()
		jsonFiles := []string{"microservices.json", "registries.json", "edge_resources.json"}
		for _, filename := range jsonFiles {
			filePath := filepath.Join(utils.ConfigDir, filename)
			if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
				logging.LogWarn(moduleName, fmt.Sprintf("Error deleting cache file %s: %v", filename, err))
			} else if err == nil {
				logging.LogDebug(moduleName, fmt.Sprintf("Deleted cache file: %s", filename))
			}
		}
	}()

	// Notify modules AFTER configuration is cleared (matching Java: notifyModules())
	func() {
		defer func() {
			if r := recover(); r != nil {
				logging.LogWarn(moduleName, fmt.Sprintf("Some module notifications failed during deprovisioning: %v", r))
			}
		}()
		logging.LogDebug(moduleName, "Notifying modules after configuration update")
		// Call callbacks to notify modules
		if fa.onMicroservicesUpdate != nil {
			if err := fa.onMicroservicesUpdate([]*models.Microservice{}); err != nil {
				logging.LogWarn(moduleName, fmt.Sprintf("Error notifying microservices update: %v", err))
			}
		}
		if fa.onRegistriesUpdate != nil {
			if err := fa.onRegistriesUpdate([]*models.Registry{}); err != nil {
				logging.LogWarn(moduleName, fmt.Sprintf("Error notifying registries update: %v", err))
			}
		}
		if fa.onRoutesUpdate != nil {
			if err := fa.onRoutesUpdate(map[string]*models.Route{}); err != nil {
				logging.LogWarn(moduleName, fmt.Sprintf("Error notifying routes update: %v", err))
			}
		}
		if fa.onConfigsUpdate != nil {
			if err := fa.onConfigsUpdate(map[string]string{}); err != nil {
				logging.LogWarn(moduleName, fmt.Sprintf("Error notifying configs update: %v", err))
			}
		}
		logging.LogDebug(moduleName, "Module notification completed")
	}()

	// Set state (matching Java)
	fa.state.SetControllerStatus(models.ControllerStatusNotProvisioned)
	fa.state.SetControllerVerified(false)

	resultMessage := "Success - cleaned up locally"
	if deprovisionRequestSuccessful {
		resultMessage = "Success - deprovisioned from controller and cleaned up locally"
	} else if !clearCredentials {
		resultMessage = "Success - cleaned up locally (controller deprovision failed)"
	}

	logging.LogInfo(moduleName, fmt.Sprintf("Finished Deprovisioning : %s", resultMessage))
	return nil
}

// Update updates the FieldAgent when configuration changes
func (fa *FieldAgent) Update() error {
	logging.LogDebug(moduleName, "Updating Field Agent due to config change")

	fa.mu.Lock()
	// No defer here because we unlock explicitly before postFogConfig

	// Reset JWT manager to ensure it picks up new credentials from reloaded config
	// This is critical when config is reloaded after provisioning
	auth.GetJWTManager().Reset()

	apiClient, err := NewAPIClient()
	if err != nil {
		return fmt.Errorf("failed to recreate API client: %w", err)
	}

	// Replace the old API client
	fa.apiClient = apiClient

	// Update orchestrator with new API client
	if fa.orchestrator != nil {
		fa.orchestrator = NewOrchestrator(apiClient)
	}

	// Release lock before making network call to avoid blocking other operations (like info command)
	fa.mu.Unlock()

	// Post fog config to controller using the NEW client
	// This ensures we connect to the correct (new) URL
	// Asynchronously post config to controller to prevent blocking local operations
	go func() {
		logging.LogDebug(moduleName, "Starting asynchronous postFogConfig")
		if err := fa.postFogConfig(); err != nil {
			logging.LogError(moduleName, "Failed to post updated fog config", err)
			// Don't fail the update for this, just log it
		} else {
			logging.LogDebug(moduleName, "Successfully posted fog config")
		}
	}()

	logging.LogDebug(moduleName, "Field Agent updated successfully")
	return nil
}

// GetActiveExecSession returns the active exec session ID for a microservice
func (fa *FieldAgent) GetActiveExecSession(microserviceUUID string) string {
	fa.execSessionsMu.RLock()
	defer fa.execSessionsMu.RUnlock()
	return fa.activeExecSessions[microserviceUUID]
}

// SetActiveExecSession sets the active exec session ID for a microservice
func (fa *FieldAgent) SetActiveExecSession(microserviceUUID, execID string) {
	fa.execSessionsMu.Lock()
	defer fa.execSessionsMu.Unlock()
	fa.activeExecSessions[microserviceUUID] = execID
	logging.LogDebug(moduleName, fmt.Sprintf("Set active exec session: microservice=%s, execID=%s", microserviceUUID, execID))
}

// RemoveActiveExecSession removes the active exec session for a microservice
func (fa *FieldAgent) RemoveActiveExecSession(microserviceUUID string) {
	fa.execSessionsMu.Lock()
	defer fa.execSessionsMu.Unlock()
	delete(fa.activeExecSessions, microserviceUUID)
	logging.LogDebug(moduleName, fmt.Sprintf("Removed active exec session: microservice=%s", microserviceUUID))
}

// HandleExecSessionClose handles closing an exec session
func (fa *FieldAgent) HandleExecSessionClose(microserviceUUID, execID string) error {
	logging.LogInfo(moduleName, fmt.Sprintf("Handling exec session close: microservice=%s, execID=%s", microserviceUUID, execID))

	// Remove from tracking
	fa.execSessionsMu.Lock()
	if currentExecID, exists := fa.activeExecSessions[microserviceUUID]; exists && currentExecID == execID {
		delete(fa.activeExecSessions, microserviceUUID)
	}
	// Close and remove callback
	if callback, exists := fa.execCallbacks[microserviceUUID]; exists {
		callback.Close()
		delete(fa.execCallbacks, microserviceUUID)
	}
	fa.execSessionsMu.Unlock()

	// Disconnect WebSocket if no other sessions (matching Java line 2515-2519)
	fa.execSessionsMu.Lock()
	hasOtherSessions := fa.activeExecSessions[microserviceUUID] != ""
	if !hasOtherSessions {
		logging.LogDebug(moduleName, "No other active sessions, cleaning up WebSocket")
		handler := fa.activeWebSockets[microserviceUUID]
		if handler != nil {
			delete(fa.activeWebSockets, microserviceUUID)
			fa.execSessionsMu.Unlock()
			handler.Disconnect()
			return nil
		}
	}
	fa.execSessionsMu.Unlock()

	return nil
}

// SendUSBInfoFromHalToController sends USB information from HAL to the controller
// This matches Java: sendUSBInfoFromHalToController() method
func (fa *FieldAgent) SendUSBInfoFromHalToController() {
	logging.LogDebug(moduleName, "Start send USB Info from hal To Controller")
	if fa.NotProvisioned() {
		return
	}

	// Get USB info from HAL
	usbInfo, err := fa.getHalResponse(halUSBInfoURL)
	if err != nil {
		logging.LogDebug(moduleName, "HAL is not enabled for this Iofog Agent at the moment")
		return
	}

	if usbInfo == "" {
		return
	}

	// Update status reporter
	statusreporter.GetInstance().UpdateResourceManagerStatus(func(status *models.ResourceManagerStatus) {
		status.SetUSBConnectionsInfo(usbInfo)
	})

	ctx, cancel := context.WithTimeout(fa.ctx, 30*time.Second)
	defer cancel()

	err = fa.apiClient.PutJSON(ctx, "hal/usb", map[string]interface{}{
		"info": usbInfo,
	})
	if err != nil {
		logging.LogError(moduleName, "Error while sending USBInfo from hal to controller", err)
	}

	logging.LogDebug(moduleName, "Finished send USB Info from hal To Controller")
}

// SendHWInfoFromHalToController sends hardware information from HAL to the controller
// This matches Java: sendHWInfoFromHalToController() method
func (fa *FieldAgent) SendHWInfoFromHalToController() {
	logging.LogDebug(moduleName, "Start send HW Info from HAL To Controller")
	if fa.NotProvisioned() {
		return
	}

	// Get HW info from HAL
	hwInfo, err := fa.getHalResponse(halHWInfoURL)
	if err != nil {
		logging.LogDebug(moduleName, "HAL is not enabled for this Iofog Agent at the moment")
		return
	}

	if hwInfo == "" {
		return
	}

	// Update status reporter
	statusreporter.GetInstance().UpdateResourceManagerStatus(func(status *models.ResourceManagerStatus) {
		status.SetHWInfo(hwInfo)
	})

	ctx, cancel := context.WithTimeout(fa.ctx, 30*time.Second)
	defer cancel()

	err = fa.apiClient.PutJSON(ctx, "hal/hw", map[string]interface{}{
		"info": hwInfo,
	})
	if err != nil {
		logging.LogError(moduleName, "Error while sending HW Info from hal to controller", err)
	}

	logging.LogDebug(moduleName, "Finished send HW Info from HAL To Controller")
}

// getHalResponse makes an HTTP GET request to HAL service and returns the response
// This matches Java: getResponse() method
func (fa *FieldAgent) getHalResponse(url string) (string, error) {
	logging.LogDebug(moduleName, "Start get response from HAL")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		logging.LogDebug(moduleName, "HAL is not enabled for this Iofog Agent at the moment")
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HAL service returned status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read HAL response: %w", err)
	}

	logging.LogDebug(moduleName, "Finished get response from HAL")
	return string(body), nil
}

// SetExecCallback sets the exec callback for a microservice
func (fa *FieldAgent) SetExecCallback(microserviceUUID string, callback *ExecSessionCallback) {
	fa.execSessionsMu.Lock()
	defer fa.execSessionsMu.Unlock()
	fa.execCallbacks[microserviceUUID] = callback
	logging.LogDebug(moduleName, fmt.Sprintf("Set exec callback for microservice: %s", microserviceUUID))
}

// GetExecCallback gets the exec callback for a microservice
func (fa *FieldAgent) GetExecCallback(microserviceUUID string) *ExecSessionCallback {
	fa.execSessionsMu.RLock()
	defer fa.execSessionsMu.RUnlock()
	return fa.execCallbacks[microserviceUUID]
}

// RemoveExecCallback removes the exec callback for a microservice
func (fa *FieldAgent) RemoveExecCallback(microserviceUUID string) {
	fa.execSessionsMu.Lock()
	defer fa.execSessionsMu.Unlock()
	if callback, exists := fa.execCallbacks[microserviceUUID]; exists {
		callback.Close()
		delete(fa.execCallbacks, microserviceUUID)
		logging.LogDebug(moduleName, fmt.Sprintf("Removed exec callback for microservice: %s", microserviceUUID))
	}
}

// HandleExecSessions handles exec session changes for microservices
func (fa *FieldAgent) HandleExecSessions(microservices []*models.Microservice) {
	logging.LogDebug(moduleName, fmt.Sprintf("Starting handleExecSessions for %d microservices", len(microservices)))

	for _, ms := range microservices {
		microserviceUUID := ms.MicroserviceUUID
		execEnabled := ms.ExecEnabled

		if !execEnabled {
			// Exec is disabled - cleanup existing session
			fa.execSessionsMu.RLock()
			existingExecID := fa.activeExecSessions[microserviceUUID]
			fa.execSessionsMu.RUnlock()

			if existingExecID != "" {
				logging.LogDebug(moduleName, fmt.Sprintf("Cleaning up exec session for microservice: %s", microserviceUUID))
				fa.HandleExecSessionClose(microserviceUUID, existingExecID)
			}
			continue
		}

		// Exec is enabled - check if session exists
		fa.execSessionsMu.RLock()
		existingExecID := fa.activeExecSessions[microserviceUUID]
		fa.execSessionsMu.RUnlock()

		if existingExecID != "" {
			// Check if session is still running
			if fa.processManager != nil {
				status, err := fa.processManager.GetExecSessionStatus(existingExecID)
				if err != nil || status == nil || !status.Running {
					// Session is not running, clean it up and create a new one
					logging.LogDebug(moduleName, fmt.Sprintf("Exec session %s is not running, cleaning up and creating new one", existingExecID))
					fa.HandleExecSessionClose(microserviceUUID, existingExecID)
					// Continue to create new session below
				} else {
					logging.LogDebug(moduleName, fmt.Sprintf("Exec session already exists and is running for microservice: %s, execID: %s", microserviceUUID, existingExecID))
					continue
				}
			} else {
				// ProcessManager not available, assume session is running
				logging.LogDebug(moduleName, fmt.Sprintf("Exec session already exists for microservice: %s (ProcessManager not available to verify)", microserviceUUID))
				continue
			}
		}

		// Create new exec session
		go func(uuid string) {
			logging.LogDebug(moduleName, fmt.Sprintf("Creating new exec session for microservice: %s", uuid))

			// Create callback
			callback := NewExecSessionCallback(uuid, "")

			// Default command: shell with fallback
			command := []string{"sh", "-c", "clear; (bash || ash || sh)"}

			// Get ProcessManager instance (we'll need to import it or get it from supervisor)
			// For now, we'll create the exec session directly via docker client
			// This is a simplified version - in full implementation, we'd go through ProcessManager
			fa.createExecSessionForMicroservice(uuid, command, callback)
		}(microserviceUUID)
	}
}

// createExecSessionForMicroservice creates an exec session for a microservice
func (fa *FieldAgent) createExecSessionForMicroservice(microserviceUUID string, command []string, callback *ExecSessionCallback) {
	logging.LogDebug(moduleName, fmt.Sprintf("Creating exec session for microservice: %s with command: %v", microserviceUUID, command))

	// Check if ProcessManager is available
	if fa.processManager == nil {
		logging.LogError(moduleName, "ProcessManager not set, cannot create exec session", fmt.Errorf("processManager is nil"))
		callback.OnError(fmt.Errorf("processManager is not available"))
		return
	}

	// Create exec session via ProcessManager
	execID, err := fa.processManager.CreateExecSession(microserviceUUID, command, callback)
	if err != nil {
		logging.LogError(moduleName, fmt.Sprintf("Failed to create exec session for microservice: %s", microserviceUUID), err)
		callback.OnError(err)
		return
	}

	// Store exec session info
	fa.execSessionsMu.Lock()
	fa.activeExecSessions[microserviceUUID] = execID
	fa.execCallbacks[microserviceUUID] = callback
	fa.execSessionsMu.Unlock()

	// Update callback with execID
	callback.SetExecID(execID)

	// Set up close handler (Matching Java logic)
	callback.SetOnCloseHandler(func() {
		logging.LogInfo(moduleName, fmt.Sprintf("Exec session closed for microservice: %s", microserviceUUID))

		fa.execSessionsMu.Lock()
		defer fa.execSessionsMu.Unlock()

		// Remove session tracking
		delete(fa.activeExecSessions, microserviceUUID)
		delete(fa.execCallbacks, microserviceUUID)

		// Disconnect and remove WebSocket handler
		if handler, exists := fa.activeWebSockets[microserviceUUID]; exists {
			logging.LogDebug(moduleName, "Disconnecting WebSocket handler due to session close")
			handler.Disconnect()
			delete(fa.activeWebSockets, microserviceUUID)
		}
	})

	// Create and connect WebSocket handler (matching Java: wsHandler.connect() after exec session creation)
	logging.LogDebug(moduleName, "Creating and connecting WebSocket handler for exec session")
	handler := GetExecSessionWebSocketHandler(microserviceUUID)
	if handler != nil {
		// Check if existing handler exists (matching Java line 2297-2302)
		fa.execSessionsMu.Lock()
		if existingHandler, exists := fa.activeWebSockets[microserviceUUID]; exists {
			logging.LogDebug(moduleName, "Found existing WebSocket handler, cleaning up before creating new one")
			existingHandler.Disconnect()
			delete(fa.activeWebSockets, microserviceUUID)
		}
		fa.execSessionsMu.Unlock()

		// Disconnect existing handler if any (matching Java behavior)
		if handler.IsConnected() {
			handler.Disconnect()
		}
		// Connect the WebSocket handler (matching Java line 2304: wsHandler.connect())
		if err := handler.Connect(); err != nil {
			logging.LogError(moduleName, fmt.Sprintf("Failed to connect WebSocket handler for exec session: %s", microserviceUUID), err)
		} else {
			// Store handler in activeWebSockets map (matching Java line 2305: activeWebSockets.put())
			fa.execSessionsMu.Lock()
			fa.activeWebSockets[microserviceUUID] = handler
			fa.execSessionsMu.Unlock()
			logging.LogDebug(moduleName, "Successfully created and connected WebSocket handler for exec session")
		}
	}

	logging.LogInfo(moduleName, fmt.Sprintf("Exec session created successfully: execID=%s, microserviceUUID=%s", execID, microserviceUUID))
}
