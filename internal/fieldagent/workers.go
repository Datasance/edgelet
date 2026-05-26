package fieldagent

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/datasance/edgelet/internal/auth"
	"github.com/datasance/edgelet/internal/config"
	"github.com/datasance/edgelet/internal/gps"
	"github.com/datasance/edgelet/internal/models"
	"github.com/datasance/edgelet/internal/network"
	"github.com/datasance/edgelet/internal/serviceaccount"
	"github.com/datasance/edgelet/internal/statusreporter"
	"github.com/datasance/edgelet/internal/utils/logging"
	"github.com/datasance/edgelet/internal/version"
)

// workerFreq returns a non-zero duration, falling back to the default if the
// configured value is zero or negative (a zero timer would fire immediately in
// a tight loop and a zero ticker panics).
func workerFreq(seconds int, defaultSeconds int) time.Duration {
	if seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	return time.Duration(defaultSeconds) * time.Second
}

const (
	pingBackoffMaxSeconds          = 300 // 5 min max backoff when controller offline
	localAPITokenRotationInterval  = 30 * time.Second
	serviceAccountRotationInterval = 30 * time.Second
)

// pingControllerWorker periodically pings the controller.
// Uses exponential backoff when controller is unreachable (edge resilience:
// avoid hammering controller when offline; agent continues with last config).
func (fa *FieldAgent) pingControllerWorker() {
	defer fa.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			logging.LogError(moduleName, "Panic recovered", fmt.Errorf("%v", r))
		}
	}()

	cfg := config.GetInstance()
	baseInterval := workerFreq(cfg.PingControllerFreqSeconds, 30)
	interval := baseInterval
	consecutiveFailures := 0

	timer := time.NewTimer(interval)
	defer timer.Stop()

	for {
		select {
		case <-fa.ctx.Done():
			return
		case <-timer.C:
			if !fa.NotProvisioned() {
				logging.LogDebug(moduleName, "Start Ping controller")
				ok := fa.ping()
				logging.LogDebug(moduleName, "Finished Ping controller")

				if ok {
					consecutiveFailures = 0
					interval = baseInterval
				} else {
					consecutiveFailures++
					// Exponential backoff: 30s, 60s, 120s, ... cap at 5 min
					backoffSec := cfg.PingControllerFreqSeconds
					if backoffSec < 30 {
						backoffSec = 30
					}
					for i := 0; i < consecutiveFailures-1 && backoffSec < pingBackoffMaxSeconds; i++ {
						backoffSec *= 2
					}
					if backoffSec > pingBackoffMaxSeconds {
						backoffSec = pingBackoffMaxSeconds
					}
					interval = time.Duration(backoffSec) * time.Second
					logging.LogInfo(moduleName, fmt.Sprintf("Controller unreachable, next ping in %v (backoff)", interval))
				}
			}
			timer.Reset(interval)
		}
	}
}

// getChangesWorker periodically gets changes from the controller.
// Uses a self-resetting timer so the interval is re-read from config on every tick.
func (fa *FieldAgent) getChangesWorker() {
	defer fa.wg.Done()

	timer := time.NewTimer(workerFreq(config.GetInstance().ChangeFrequency, 20))
	defer timer.Stop()

	for {
		select {
		case <-fa.ctx.Done():
			return
		case <-timer.C:
			logging.LogDebug(moduleName, "Start get IOFog changes list from IOFog controller")

			if fa.NotProvisioned() || !fa.IsControllerConnected(false) {
				logging.LogDebug(moduleName, "Cannot get change list due to controller status not provisioned or controller not connected")
				timer.Reset(workerFreq(config.GetInstance().ChangeFrequency, 20))
				continue
			}

			// Get changes from controller
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			result, err := fa.apiClient.Request(ctx, "config/changes", GET, nil, nil)
			cancel()

			if err != nil {
				if isCertificateError(err) {
					fa.verificationFailed(err)
					logging.LogError(moduleName, "Unable to get changes due to broken certificate", err)
				} else {
					logging.LogError(moduleName, "Unable to get changes", err)
				}
				timer.Reset(workerFreq(config.GetInstance().ChangeFrequency, 20))
				continue
			}

			// Update last command time
			fa.state.SetLastCommandTime(fa.state.GetLastGetChangesList())

			// Process changes
			lastUpdated, _ := result["lastUpdated"].(string)
			logging.LogDebug(moduleName, fmt.Sprintf("Processing changes with lastUpdated: %s", lastUpdated))

			resetChanges := fa.processChanges(result)

			// Reset changes flags if processing was successful
			if lastUpdated != "" && resetChanges {
				logging.LogDebug(moduleName, fmt.Sprintf("Resetting config changes flags with lastUpdated: %s", lastUpdated))
				ctx2, cancel2 := context.WithTimeout(context.Background(), 30*time.Second)
				err := fa.apiClient.PatchJSON(ctx2, "config/changes", map[string]interface{}{
					"lastUpdated": lastUpdated,
				})
				cancel2()

				if err != nil {
					logging.LogError(moduleName, "Resetting config changes has failed", err)
				} else {
					logging.LogDebug(moduleName, "Successfully reset config changes flags")
				}
			}

			// Update initialization flag
			fa.state.SetInitialization(fa.state.IsInitialization() && !resetChanges)
			logging.LogDebug(moduleName, fmt.Sprintf("Finished getChangesList cycle with initialization: %v", fa.state.IsInitialization()))
			timer.Reset(workerFreq(config.GetInstance().ChangeFrequency, 20))
		}
	}
}

// postStatusWorker periodically posts status to the controller.
// Uses a self-resetting timer so the interval is re-read from config on every tick.
func (fa *FieldAgent) postStatusWorker() {
	defer fa.wg.Done()

	timer := time.NewTimer(workerFreq(config.GetInstance().StatusFrequency, 10))
	defer timer.Stop()

	for {
		select {
		case <-fa.ctx.Done():
			return
		case <-timer.C:
			fa.PostStatusHelper()
			timer.Reset(workerFreq(config.GetInstance().StatusFrequency, 10))
		}
	}
}

// PostStatusHelper posts the fog status to the controller
// Exported for use by Edge Guard to immediately notify controller of hardware changes
func (fa *FieldAgent) PostStatusHelper() {
	logging.LogDebug(moduleName, "posting ioFog status")

	status := fa.getFogStatus()

	cfg := config.GetInstance()
	if cfg.Debugging {
		logging.LogInfo(moduleName, fmt.Sprintf("Status: %+v", status))
	}

	connected := fa.IsControllerConnected(false)
	if !connected {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	err := fa.putStatus(ctx, status)
	cancel()

	if err != nil {
		if isCertificateError(err) {
			fa.verificationFailed(err)
			logging.LogError(moduleName, "Unable to send status due to broken certificate", err)
		} else if isUnauthorizedError(err) {
			if !fa.NotProvisioned() {
				if depErr := fa.Deprovision(true); depErr != nil {
					logging.LogWarn(moduleName, fmt.Sprintf("Deprovision failed: %v", depErr))
				}
			}
			logging.LogError(moduleName, "Unable to send status due to unauthorized access", err)
		} else {
			logging.LogError(moduleName, "Unable to send status", err)
		}
		logging.LogDebug(moduleName, "Finished posting ioFog status")
		return
	}
	statusreporter.GetInstance().UpdateProcessManagerStatus(func(pmStatus *models.ProcessManagerStatus) {
		pmStatus.RemoveNotRunningMicroserviceStatus()
	})

	logging.LogDebug(moduleName, "Finished posting ioFog status")
}

func (fa *FieldAgent) putStatus(ctx context.Context, status map[string]interface{}) error {
	if fa.postStatusFn != nil {
		return fa.postStatusFn(ctx, status)
	}
	if fa.apiClient == nil {
		return fmt.Errorf("api client is not initialized")
	}
	return fa.apiClient.PutJSON(ctx, "status", status)
}

// getFogStatus creates the fog status report
func (fa *FieldAgent) getFogStatus() map[string]interface{} {
	logging.LogDebug(moduleName, "get Fog Status")

	// Get StatusReporter instance to get status from all modules
	statusReporter := statusreporter.GetInstance()

	supervisorStatus := statusReporter.GetSupervisorStatus()
	resourceConsumptionStatus := statusReporter.GetResourceConsumptionManagerStatus()
	processManagerStatus := statusReporter.GetProcessManagerStatus()
	fieldAgentStatus := statusReporter.GetFieldAgentStatus()
	statusReporterStatus := statusReporter.GetStatusReporterStatus()
	sshManagerStatus := statusReporter.GetSSHProxyManagerStatus()
	volumeMountStatus := statusReporter.GetVolumeMountManagerStatus()

	// Get daemon status string
	daemonStatusStr := "UNKNOWN"
	if supervisorStatus.DaemonStatus != "" {
		daemonStatusStr = string(supervisorStatus.DaemonStatus)
	}

	// Get microservice status JSON
	microserviceStatusJSON := processManagerStatus.GetJSONMicroservicesStatus()
	if microserviceStatusJSON == "" {
		microserviceStatusJSON = "[]"
	}

	// Get repository status JSON
	repositoryStatusJSON := processManagerStatus.GetJSONRegistriesStatus()
	if repositoryStatusJSON == "" {
		repositoryStatusJSON = "[]"
	}

	// Get tunnel status JSON
	tunnelStatusJSON := sshManagerStatus.GetJSONProxyStatus()
	if tunnelStatusJSON == "" {
		tunnelStatusJSON = "{}"
	}

	// Get warning message
	warningMessage := supervisorStatus.WarningMessage
	if warningMessage == "" {
		warningMessage = ""
	}
	controllerRuntimes := runtimeNamesForController(
		fa.config.ContainerEngine,
		statusreporter.GetAvailableRuntimes(),
	)

	status := map[string]interface{}{
		"daemonStatus":              daemonStatusStr,
		"daemonOperatingDuration":   supervisorStatus.GetOperationDuration(),
		"daemonLastStart":           supervisorStatus.DaemonLastStart,
		"warningMessage":            warningMessage,
		"memoryUsage":               resourceConsumptionStatus.MemoryUsage,
		"diskUsage":                 resourceConsumptionStatus.DiskUsage,
		"cpuUsage":                  resourceConsumptionStatus.CPUUsage,
		"memoryViolation":           resourceConsumptionStatus.MemoryViolation,
		"diskViolation":             resourceConsumptionStatus.DiskViolation,
		"cpuViolation":              resourceConsumptionStatus.CPUViolation,
		"systemAvailableDisk":       float64(resourceConsumptionStatus.AvailableDisk),
		"systemAvailableMemory":     float64(resourceConsumptionStatus.AvailableMemory),
		"systemTotalCpu":            resourceConsumptionStatus.TotalCPU,
		"microserviceStatus":        microserviceStatusJSON,
		"repositoryCount":           len(processManagerStatus.GetRegistriesStatus()),
		"repositoryStatus":          repositoryStatusJSON,
		"systemTime":                statusReporterStatus.SystemTime,
		"lastStatusTime":            statusReporterStatus.LastUpdate,
		"ipAddress":                 network.GetInstance().GetCurrentIPAddress(), // Get from NetworkInterfaceManager
		"ipAddressExternal":         fa.config.IPAddressExternal,
		"microserviceMessageCounts": "[]",
		"lastCommandTime":           fieldAgentStatus.LastCommandTime,
		"tunnelStatus":              tunnelStatusJSON,
		"version":                   version.GetVersion(), // Version from build-time ldflags
		"isReadyToUpgrade":          fieldAgentStatus.ReadyToUpgrade,
		"isReadyToRollback":         fieldAgentStatus.ReadyToRollback,
		"activeVolumeMounts":        volumeMountStatus.ActiveMounts,
		"volumeMountLastUpdate":     volumeMountStatus.LastUpdate,
		"gpsStatus":                 string(gps.GetInstance().GetStatus().GetHealthStatus()), // Get from GpsManager
		"availableRuntimes":         controllerRuntimes,
	}

	return status
}

func runtimeNamesForController(_ string, available []string) []string {
	filteredSet := make(map[string]struct{}, len(available))
	for _, runtimeName := range available {
		name := strings.TrimSpace(runtimeName)
		if name == "" {
			continue
		}
		filteredSet[name] = struct{}{}
	}
	filtered := make([]string, 0, len(filteredSet))
	for name := range filteredSet {
		filtered = append(filtered, name)
	}
	sort.Strings(filtered)
	return filtered
}

// isUnauthorizedError checks if an error is an unauthorized error
func isUnauthorizedError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(strings.ToLower(errStr), "unauthorized") ||
		strings.Contains(strings.ToLower(errStr), "401")
}

func (fa *FieldAgent) localAPITokenRotationWorker() {
	defer fa.wg.Done()
	timer := time.NewTimer(localAPITokenRotationInterval)
	defer timer.Stop()

	for {
		select {
		case <-fa.ctx.Done():
			return
		case <-timer.C:
			token, err := auth.GetLocalTokenManager().LoadToken()
			if err != nil {
				if reconcileErr := auth.EnsureEdgeletAPITokenForCurrentState(); reconcileErr != nil {
					logging.LogWarn(moduleName, fmt.Sprintf("edgelet-api token reconcile failed: %v", reconcileErr))
				}
				timer.Reset(localAPITokenRotationInterval)
				continue
			}
			rotate, err := auth.ShouldRotateEdgeletAPIToken(token, time.Now())
			if err != nil || rotate {
				if reconcileErr := auth.EnsureEdgeletAPITokenForCurrentState(); reconcileErr != nil {
					logging.LogWarn(moduleName, fmt.Sprintf("edgelet-api token rotation failed: %v", reconcileErr))
				}
			}
			timer.Reset(localAPITokenRotationInterval)
		}
	}
}

func (fa *FieldAgent) serviceAccountTokenRotationWorker() {
	defer fa.wg.Done()
	timer := time.NewTimer(serviceAccountRotationInterval)
	defer timer.Stop()

	for {
		select {
		case <-fa.ctx.Done():
			return
		case <-timer.C:
			if !fa.NotProvisioned() {
				if err := serviceaccount.GetInstance().RotateExpiringManagedTokens(fa.GetLatestMicroservices(), time.Now()); err != nil {
					logging.LogWarn(moduleName, fmt.Sprintf("service-account token rotation failed: %v", err))
				}
			}
			timer.Reset(serviceAccountRotationInterval)
		}
	}
}
