package fieldagent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/eclipse-iofog/agent-go/internal/config"
	"github.com/eclipse-iofog/agent-go/internal/diagnostics"
	"github.com/eclipse-iofog/agent-go/internal/gps"
	"github.com/eclipse-iofog/agent-go/internal/network"
	"github.com/eclipse-iofog/agent-go/internal/statusreporter"
	"github.com/eclipse-iofog/agent-go/internal/utils/logging"
	"github.com/eclipse-iofog/agent-go/internal/version"
)

// pingControllerWorker periodically pings the controller
func (fa *FieldAgent) pingControllerWorker() {
	defer fa.wg.Done()

	cfg := config.GetInstance()
	ticker := time.NewTicker(time.Duration(cfg.PingControllerFreqSeconds) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-fa.ctx.Done():
			return
		case <-ticker.C:
			logging.LogDebug(moduleName, "Start Ping controller")
			fa.ping()
			logging.LogDebug(moduleName, "Finished Ping controller")
		}
	}
}

// getChangesWorker periodically gets changes from the controller
func (fa *FieldAgent) getChangesWorker() {
	defer fa.wg.Done()

	cfg := config.GetInstance()
	ticker := time.NewTicker(time.Duration(cfg.ChangeFrequency) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-fa.ctx.Done():
			return
		case <-ticker.C:
			logging.LogDebug(moduleName, "Start get IOFog changes list from IOFog controller")

			if fa.NotProvisioned() || !fa.IsControllerConnected(false) {
				logging.LogDebug(moduleName, "Cannot get change list due to controller status not provisioned or controller not connected")
				continue
			}

			// Get changes from controller
			ctx, cancel := context.WithTimeout(fa.ctx, 30*time.Second)
			result, err := fa.apiClient.Request(ctx, "config/changes", GET, nil, nil)
			cancel()

			if err != nil {
				if isCertificateError(err) {
					fa.verificationFailed(err)
					logging.LogError(moduleName, "Unable to get changes due to broken certificate", err)
				} else {
					logging.LogError(moduleName, "Unable to get changes", err)
				}
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
				ctx, cancel := context.WithTimeout(fa.ctx, 30*time.Second)
				err := fa.apiClient.PatchJSON(ctx, "config/changes", map[string]interface{}{
					"lastUpdated": lastUpdated,
				})
				cancel()

				if err != nil {
					logging.LogError(moduleName, "Resetting config changes has failed", err)
				} else {
					logging.LogDebug(moduleName, "Successfully reset config changes flags")
				}
			}

			// Update initialization flag
			fa.state.SetInitialization(fa.state.IsInitialization() && !resetChanges)
			logging.LogDebug(moduleName, fmt.Sprintf("Finished getChangesList cycle with initialization: %v", fa.state.IsInitialization()))
		}
	}
}

// postStatusWorker periodically posts status to the controller
func (fa *FieldAgent) postStatusWorker() {
	defer fa.wg.Done()

	cfg := config.GetInstance()
	ticker := time.NewTicker(time.Duration(cfg.StatusFrequency) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-fa.ctx.Done():
			return
		case <-ticker.C:
			fa.PostStatusHelper()
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

	ctx, cancel := context.WithTimeout(fa.ctx, 30*time.Second)
	err := fa.apiClient.PutJSON(ctx, "status", status)
	cancel()

	if err != nil {
		if isCertificateError(err) {
			fa.verificationFailed(err)
			logging.LogError(moduleName, "Unable to send status due to broken certificate", err)
		} else if isUnauthorizedError(err) {
			fa.Deprovision(true)
			logging.LogError(moduleName, "Unable to send status due to unauthorized access", err)
		} else {
			logging.LogError(moduleName, "Unable to send status", err)
		}
	} else {
		// On success, notify other modules if needed
		// This would be implemented when StatusReporter is available
	}

	logging.LogDebug(moduleName, "Finished posting ioFog status")
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
	messageBusStatus := statusReporter.GetMessageBusStatus()
	sshManagerStatus := statusReporter.GetSshProxyManagerStatus()
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

	// Get message counts JSON
	messageCountsJSON := messageBusStatus.GetJSONPublishedMessagesPerMicroservice()
	if messageCountsJSON == "" {
		messageCountsJSON = "[]"
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
		"processedMessages":         messageBusStatus.ProcessedMessages,
		"microserviceMessageCounts": messageCountsJSON,
		"messageSpeed":              messageBusStatus.AverageSpeed,
		"lastCommandTime":           fieldAgentStatus.LastCommandTime,
		"tunnelStatus":              tunnelStatusJSON,
		"version":                   version.GetVersion(), // Version from build-time ldflags
		"isReadyToUpgrade":          fieldAgentStatus.ReadyToUpgrade,
		"isReadyToRollback":         fieldAgentStatus.ReadyToRollback,
		"activeVolumeMounts":        volumeMountStatus.ActiveMounts,
		"volumeMountLastUpdate":     volumeMountStatus.LastUpdate,
		"gpsStatus":                 string(gps.GetInstance().GetStatus().GetHealthStatus()), // Get from GpsManager
	}

	return status
}

// postDiagnosticsWorker periodically posts diagnostics (strace data) to the controller
func (fa *FieldAgent) postDiagnosticsWorker() {
	defer fa.wg.Done()

	cfg := config.GetInstance()
	ticker := time.NewTicker(time.Duration(cfg.PostDiagnosticsFreq) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-fa.ctx.Done():
			return
		case <-ticker.C:
			fa.postDiagnosticsHelper()
		}
	}
}

// postDiagnosticsHelper posts strace diagnostics to the controller
func (fa *FieldAgent) postDiagnosticsHelper() {
	logging.LogDebug(moduleName, "Start posting diagnostic")

	// Import diagnostics package here to avoid circular dependency
	straceManager := diagnostics.GetStraceInstance()
	monitoringMicroservices := straceManager.GetMonitoringMicroservices()

	if len(monitoringMicroservices) == 0 {
		logging.LogDebug(moduleName, "No microservices to monitor, skipping diagnostics post")
		return
	}

	// Build strace data array
	straceDataArray := make([]map[string]interface{}, 0)
	for _, microservice := range monitoringMicroservices {
		straceDataArray = append(straceDataArray, map[string]interface{}{
			"microserviceUuid": microservice.GetMicroserviceUUID(),
			"buffer":            microservice.GetResultBufferAsString(),
		})
		// Clear buffer after reading
		microservice.ClearResultBuffer()
	}

	// Build request body
	requestBody := map[string]interface{}{
		"straceData": straceDataArray,
	}

	// Post to controller
	ctx, cancel := context.WithTimeout(fa.ctx, 30*time.Second)
	err := fa.apiClient.PutJSON(ctx, "strace", requestBody)
	cancel()

	if err != nil {
		logging.LogError(moduleName, "Unable send strace logs", err)
	} else {
		logging.LogDebug(moduleName, "Successfully posted strace diagnostics")
	}

	logging.LogDebug(moduleName, "Finished posting diagnostic")
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
