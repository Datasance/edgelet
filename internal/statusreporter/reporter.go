package statusreporter

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/eclipse-iofog/agent-go/internal/config"
	"github.com/eclipse-iofog/agent-go/internal/models"
	"github.com/eclipse-iofog/agent-go/internal/utils"
	"github.com/eclipse-iofog/agent-go/internal/utils/logging"
)

const (
	moduleName = "Status Reporter"
	// Number of modules (from Java Constants.NUMBER_OF_MODULES)
	numberOfModules = 9
)

// StatusReporter aggregates and reports status from all modules
type StatusReporter struct {
	config *config.Config
	mu     sync.RWMutex

	// Status objects for each module
	supervisorStatus                *models.SupervisorStatus
	resourceConsumptionManagerStatus *models.ResourceConsumptionManagerStatus
	resourceManagerStatus           *models.ResourceManagerStatus
	fieldAgentStatus                *models.FieldAgentStatus
	statusReporterStatus            *models.StatusReporterStatus
	processManagerStatus            *models.ProcessManagerStatus
	localApiStatus                  *models.LocalApiStatus
	messageBusStatus                *models.MessageBusStatus
	sshProxyManagerStatus           *models.SshProxyManagerStatus
	volumeMountManagerStatus        *models.VolumeMountManagerStatus

	// Context for background workers
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

var (
	instance *StatusReporter
	once     sync.Once
)

// GetInstance returns the singleton StatusReporter instance
func GetInstance() *StatusReporter {
	once.Do(func() {
		instance = &StatusReporter{
			config:                         config.GetInstance(),
			supervisorStatus:                models.NewSupervisorStatus(numberOfModules),
			resourceConsumptionManagerStatus: models.NewResourceConsumptionManagerStatus(),
			resourceManagerStatus:          models.NewResourceManagerStatus(),
			fieldAgentStatus:                models.NewFieldAgentStatus(),
			statusReporterStatus:            models.NewStatusReporterStatus(),
			processManagerStatus:            models.NewProcessManagerStatus(),
			localApiStatus:                  models.NewLocalApiStatus(),
			messageBusStatus:                models.NewMessageBusStatus(),
			sshProxyManagerStatus:           models.NewSshProxyManagerStatus(),
			volumeMountManagerStatus:        models.NewVolumeMountManagerStatus(),
		}
	})
	return instance
}

// Start starts the StatusReporter
func (sr *StatusReporter) Start() error {
	logging.LogInfo(moduleName, "Starting Status Reporter")

	// Create context for cancellation
	sr.ctx, sr.cancel = context.WithCancel(context.Background())

	// Start background worker to update system time
	sr.wg.Add(1)
	go sr.setSystemTimeWorker()

	logging.LogInfo(moduleName, "Started Status Reporter")
	return nil
}

// Stop stops the StatusReporter
func (sr *StatusReporter) Stop() error {
	logging.LogDebug(moduleName, "Stopping Status Reporter")

	if sr.cancel != nil {
		sr.cancel()
	}

	// Wait for all workers to finish
	sr.wg.Wait()

	logging.LogDebug(moduleName, "Status Reporter stopped")
	return nil
}

// setSystemTimeWorker periodically updates the system time in status
func (sr *StatusReporter) setSystemTimeWorker() {
	defer sr.wg.Done()

	cfg := sr.config
	ticker := time.NewTicker(time.Duration(cfg.SetSystemTimeFreqSeconds) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-sr.ctx.Done():
			return
		case <-ticker.C:
			logging.LogDebug(moduleName, "Inside setStatusReporterSystemTime")
			sr.mu.Lock()
			now := time.Now().UnixMilli()
			sr.statusReporterStatus.SetSystemTime(now)
			sr.mu.Unlock()
			logging.LogDebug(moduleName, "Finished setStatusReporterSystemTime")
		}
	}
}

// GetStatusReport returns a formatted status report string (for CLI)
func (sr *StatusReporter) GetStatusReport() string {
	logging.LogInfo(moduleName, "Start Getting Status Report")

	sr.mu.RLock()
	defer sr.mu.RUnlock()

	var result string
	diskUsage := sr.resourceConsumptionManagerStatus.DiskUsage
	availableDisk := float64(sr.resourceConsumptionManagerStatus.AvailableDisk) / 1024.0 / 1024.0
	availableMemory := float64(sr.resourceConsumptionManagerStatus.AvailableMemory) / 1024.0 / 1024.0
	totalCpu := sr.resourceConsumptionManagerStatus.TotalCPU
	memoryUsage := sr.resourceConsumptionManagerStatus.MemoryUsage
	cpuUsage := sr.resourceConsumptionManagerStatus.CPUUsage

	// Debug logging to trace status values (matching Java debug logging)
	logging.LogDebug(moduleName, fmt.Sprintf("Status values: MemoryUsage=%.2f MiB, CPUUsage=%.2f%%, DiskUsage=%.2f GiB, AvailableMemory=%.2f MB, AvailableDisk=%.2f MB, TotalCPU=%.2f%%",
		memoryUsage, cpuUsage, diskUsage, availableMemory, availableDisk, totalCpu))

	// Get connection status (matching Java: getStatusReport())
	connectionStatus := "not connected"
	currentStatus := sr.fieldAgentStatus.ControllerStatus
	logging.LogDebug(moduleName, fmt.Sprintf("Current controller status from StatusReporter: %s (ControllerVerified: %v)", 
		currentStatus, sr.fieldAgentStatus.ControllerVerified))
	
	switch currentStatus {
	case models.ControllerStatusNotProvisioned:
		connectionStatus = "not provisioned"
	case models.ControllerStatusBrokenCertificate:
		connectionStatus = "broken certificate"
	case models.ControllerStatusNotConnected:
		connectionStatus = "not connected"
	case models.ControllerStatusOK:
		connectionStatus = "ok" // Matching Java: case OK: connectionStatus = "ok";
	default:
		// Default to "not connected" if status is unknown
		logging.LogDebug(moduleName, fmt.Sprintf("Unknown controller status: %s, defaulting to 'not connected'", currentStatus))
		connectionStatus = "not connected"
	}
	
	logging.LogDebug(moduleName, fmt.Sprintf("Connection status for report: %s", connectionStatus))

	// Format system time
	systemTime := time.UnixMilli(sr.statusReporterStatus.SystemTime)
	dateFormat := systemTime.Format("02/01/2006 03:04 PM")

	// Get daemon status (matching Java: supervisorStatus.getDaemonStatus().name())
	daemonStatus := string(sr.supervisorStatus.DaemonStatus)
	if daemonStatus == "" {
		// Default to RUNNING if not set (shouldn't happen, but safety check)
		daemonStatus = "RUNNING"
	}
	result += fmt.Sprintf("ioFog daemon                : %s\n", daemonStatus)
	result += fmt.Sprintf("Memory Usage                : about %.2f MiB\n", memoryUsage)
	if diskUsage < 1 {
		result += fmt.Sprintf("Disk Usage                  : about %.2f MiB\n", diskUsage*1024)
	} else {
		result += fmt.Sprintf("Disk Usage                  : about %.2f GiB\n", diskUsage)
	}
	result += fmt.Sprintf("CPU Usage                   : about %.2f %%\n", cpuUsage)
	result += fmt.Sprintf("Running Microservices       : %d\n", sr.processManagerStatus.RunningMicroservicesCount)
	result += fmt.Sprintf("Connection to Controller    : %s\n", connectionStatus)
	result += fmt.Sprintf("Messages Processed          : about %d\n", sr.messageBusStatus.ProcessedMessages)
	result += fmt.Sprintf("System Time                 : %s\n", dateFormat)

	// Calculate total disk for percentage
	totalDisk := float64(sr.resourceConsumptionManagerStatus.TotalDiskSpace) / 1024.0 / 1024.0
	diskPercent := 0.0
	if totalDisk > 0 {
		diskPercent = (availableDisk / totalDisk) * 100.0
	}

	result += fmt.Sprintf("System Available Disk       : %.2f MB (%.2f %%)\n", availableDisk, diskPercent)
	result += fmt.Sprintf("System Available Memory     : %.2f MB\n", availableMemory)
	result += fmt.Sprintf("System Total CPU            : %.2f %%\n", totalCpu)

	logging.LogDebug(moduleName, "Finished Getting Status Report")
	return result
}

// Status getters (thread-safe)

// GetSupervisorStatus returns the supervisor status
func (sr *StatusReporter) GetSupervisorStatus() *models.SupervisorStatus {
	sr.mu.RLock()
	defer sr.mu.RUnlock()
	return sr.supervisorStatus
}

// GetResourceConsumptionManagerStatus returns the resource consumption manager status
func (sr *StatusReporter) GetResourceConsumptionManagerStatus() *models.ResourceConsumptionManagerStatus {
	sr.mu.RLock()
	defer sr.mu.RUnlock()
	return sr.resourceConsumptionManagerStatus
}

// GetResourceManagerStatus returns the resource manager status
func (sr *StatusReporter) GetResourceManagerStatus() *models.ResourceManagerStatus {
	sr.mu.RLock()
	defer sr.mu.RUnlock()
	return sr.resourceManagerStatus
}

// GetFieldAgentStatus returns the field agent status
func (sr *StatusReporter) GetFieldAgentStatus() *models.FieldAgentStatus {
	sr.mu.RLock()
	defer sr.mu.RUnlock()
	return sr.fieldAgentStatus
}

// GetStatusReporterStatus returns the status reporter status
func (sr *StatusReporter) GetStatusReporterStatus() *models.StatusReporterStatus {
	sr.mu.RLock()
	defer sr.mu.RUnlock()
	return sr.statusReporterStatus
}

// GetProcessManagerStatus returns the process manager status
func (sr *StatusReporter) GetProcessManagerStatus() *models.ProcessManagerStatus {
	sr.mu.RLock()
	defer sr.mu.RUnlock()
	return sr.processManagerStatus
}

// GetLocalApiStatus returns the local API status
func (sr *StatusReporter) GetLocalApiStatus() *models.LocalApiStatus {
	sr.mu.RLock()
	defer sr.mu.RUnlock()
	return sr.localApiStatus
}

// GetMessageBusStatus returns the message bus status
func (sr *StatusReporter) GetMessageBusStatus() *models.MessageBusStatus {
	sr.mu.RLock()
	defer sr.mu.RUnlock()
	return sr.messageBusStatus
}

// GetSshProxyManagerStatus returns the SSH proxy manager status
func (sr *StatusReporter) GetSshProxyManagerStatus() *models.SshProxyManagerStatus {
	sr.mu.RLock()
	defer sr.mu.RUnlock()
	return sr.sshProxyManagerStatus
}

// GetVolumeMountManagerStatus returns the volume mount manager status
func (sr *StatusReporter) GetVolumeMountManagerStatus() *models.VolumeMountManagerStatus {
	sr.mu.RLock()
	defer sr.mu.RUnlock()
	return sr.volumeMountManagerStatus
}

// Status setters (thread-safe, return status for chaining)

// UpdateSupervisorStatus updates the supervisor status securely
func (sr *StatusReporter) UpdateSupervisorStatus(fn func(*models.SupervisorStatus)) {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	sr.statusReporterStatus.SetLastUpdate(time.Now().UnixMilli())
	fn(sr.supervisorStatus)
}

// UpdateResourceConsumptionManagerStatus updates the resource consumption manager status securely
func (sr *StatusReporter) UpdateResourceConsumptionManagerStatus(fn func(*models.ResourceConsumptionManagerStatus)) {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	sr.statusReporterStatus.SetLastUpdate(time.Now().UnixMilli())
	fn(sr.resourceConsumptionManagerStatus)
}

// UpdateResourceManagerStatus updates the resource manager status securely
func (sr *StatusReporter) UpdateResourceManagerStatus(fn func(*models.ResourceManagerStatus)) {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	sr.statusReporterStatus.SetLastUpdate(time.Now().UnixMilli())
	fn(sr.resourceManagerStatus)
}

// UpdateFieldAgentStatus updates the field agent status securely
func (sr *StatusReporter) UpdateFieldAgentStatus(fn func(*models.FieldAgentStatus)) {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	sr.statusReporterStatus.SetLastUpdate(time.Now().UnixMilli())
	fn(sr.fieldAgentStatus)
}

// UpdateStatusReporterStatus updates the status reporter status securely
func (sr *StatusReporter) UpdateStatusReporterStatus(fn func(*models.StatusReporterStatus)) {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	sr.statusReporterStatus.SetLastUpdate(time.Now().UnixMilli())
	fn(sr.statusReporterStatus)
}

// UpdateProcessManagerStatus updates the process manager status securely
func (sr *StatusReporter) UpdateProcessManagerStatus(fn func(*models.ProcessManagerStatus)) {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	sr.statusReporterStatus.SetLastUpdate(time.Now().UnixMilli())
	fn(sr.processManagerStatus)
}

// UpdateLocalApiStatus updates the local API status securely
func (sr *StatusReporter) UpdateLocalApiStatus(fn func(*models.LocalApiStatus)) {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	sr.statusReporterStatus.SetLastUpdate(time.Now().UnixMilli())
	fn(sr.localApiStatus)
}

// UpdateMessageBusStatus updates the message bus status securely
func (sr *StatusReporter) UpdateMessageBusStatus(fn func(*models.MessageBusStatus)) {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	sr.statusReporterStatus.SetLastUpdate(time.Now().UnixMilli())
	fn(sr.messageBusStatus)
}

// UpdateSshProxyManagerStatus updates the SSH proxy manager status securely
func (sr *StatusReporter) UpdateSshProxyManagerStatus(fn func(*models.SshProxyManagerStatus)) {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	sr.statusReporterStatus.SetLastUpdate(time.Now().UnixMilli())
	fn(sr.sshProxyManagerStatus)
}

// UpdateVolumeMountManagerStatus updates the volume mount manager status
func (sr *StatusReporter) UpdateVolumeMountManagerStatus(activeMounts int64, lastUpdate int64) {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	sr.volumeMountManagerStatus.SetActiveMounts(activeMounts)
	sr.volumeMountManagerStatus.SetLastUpdate(lastUpdate)
	sr.statusReporterStatus.SetLastUpdate(time.Now().UnixMilli())
}

// GetName returns the module name
func (sr *StatusReporter) GetName() string {
	return moduleName
}

// GetModuleIndex returns the module index
func (sr *StatusReporter) GetModuleIndex() int {
	return utils.StatusReporter
}
