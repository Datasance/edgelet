package statusreporter

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/eclipse-iofog/agent/internal/config"
	"github.com/eclipse-iofog/agent/internal/models"
	"github.com/eclipse-iofog/agent/internal/utils"
	"github.com/eclipse-iofog/agent/internal/utils/logging"
)

const (
	moduleName = "Status Reporter"
	// Number of modules (from Java Constants.NUMBER_OF_MODULES)
	numberOfModules = 8
)

// StatusReporter aggregates and reports status from all modules
type StatusReporter struct {
	config *config.Config
	mu     sync.RWMutex

	// Status objects for each module
	supervisorStatus                 *models.SupervisorStatus
	resourceConsumptionManagerStatus *models.ResourceConsumptionManagerStatus
	resourceManagerStatus            *models.ResourceManagerStatus
	fieldAgentStatus                 *models.FieldAgentStatus
	statusReporterStatus             *models.StatusReporterStatus
	processManagerStatus             *models.ProcessManagerStatus
	localAPIStatus                   *models.LocalAPIStatus
	sshProxyManagerStatus            *models.SSHProxyManagerStatus
	volumeMountManagerStatus         *models.VolumeMountManagerStatus

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
			config:                           config.GetInstance(),
			supervisorStatus:                 models.NewSupervisorStatus(numberOfModules),
			resourceConsumptionManagerStatus: models.NewResourceConsumptionManagerStatus(),
			resourceManagerStatus:            models.NewResourceManagerStatus(),
			fieldAgentStatus:                 models.NewFieldAgentStatus(),
			statusReporterStatus:             models.NewStatusReporterStatus(),
			processManagerStatus:             models.NewProcessManagerStatus(),
			localAPIStatus:                   models.NewLocalAPIStatus(),
			sshProxyManagerStatus:            models.NewSSHProxyManagerStatus(),
			volumeMountManagerStatus:         models.NewVolumeMountManagerStatus(),
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
	defer func() {
		if r := recover(); r != nil {
			logging.LogError(moduleName, "Panic recovered", fmt.Errorf("%v", r))
		}
	}()

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
	totalCPU := sr.resourceConsumptionManagerStatus.TotalCPU
	memoryUsage := sr.resourceConsumptionManagerStatus.MemoryUsage
	cpuUsage := sr.resourceConsumptionManagerStatus.CPUUsage

	// Debug logging to trace status values (matching Java debug logging)
	logging.LogDebug(moduleName, fmt.Sprintf("Status values: MemoryUsage=%.2f MiB, CPUUsage=%.2f%%, DiskUsage=%.2f GiB, AvailableMemory=%.2f MB, AvailableDisk=%.2f MB, TotalCPU=%.2f%%",
		memoryUsage, cpuUsage, diskUsage, availableMemory, availableDisk, totalCPU))

	// Get connection status (matching Java: getStatusReport())
	var connectionStatus string
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
	result += fmt.Sprintf("System Time                 : %s\n", dateFormat)

	// Calculate total disk for percentage
	totalDisk := float64(sr.resourceConsumptionManagerStatus.TotalDiskSpace) / 1024.0 / 1024.0
	diskPercent := 0.0
	if totalDisk > 0 {
		diskPercent = (availableDisk / totalDisk) * 100.0
	}

	result += fmt.Sprintf("System Available Disk       : %.2f MB (%.2f %%)\n", availableDisk, diskPercent)
	result += fmt.Sprintf("System Available Memory     : %.2f MB\n", availableMemory)
	result += fmt.Sprintf("System Total CPU            : %.2f %%\n", totalCPU)
	availableInterfaces := getAvailableNetworkInterfaces()
	availableInterfacesLine := "none"
	if len(availableInterfaces) > 0 {
		availableInterfacesLine = strings.Join(availableInterfaces, ", ")
	}
	result += fmt.Sprintf("Available Network Interfaces : %s\n", availableInterfacesLine)

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

func getAvailableNetworkInterfaces() []string {
	interfaces, err := net.Interfaces()
	if err != nil {
		logging.LogWarn(moduleName, fmt.Sprintf("Unable to list network interfaces for status report: %v", err))
		return nil
	}

	names := make([]string, 0, len(interfaces))
	for _, iface := range interfaces {
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		name := strings.TrimSpace(iface.Name)
		if name == "" {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// GetProcessManagerStatus returns the process manager status
func (sr *StatusReporter) GetProcessManagerStatus() *models.ProcessManagerStatus {
	sr.mu.RLock()
	defer sr.mu.RUnlock()
	return sr.processManagerStatus
}

// GetLocalAPIStatus returns the local API status
func (sr *StatusReporter) GetLocalAPIStatus() *models.LocalAPIStatus {
	sr.mu.RLock()
	defer sr.mu.RUnlock()
	return sr.localAPIStatus
}

// GetSSHProxyManagerStatus returns the SSH proxy manager status
func (sr *StatusReporter) GetSSHProxyManagerStatus() *models.SSHProxyManagerStatus {
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

// ResetProcessManagerStatus clears all process-manager status data.
func (sr *StatusReporter) ResetProcessManagerStatus() {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	sr.statusReporterStatus.SetLastUpdate(time.Now().UnixMilli())
	sr.processManagerStatus.ClearMicroserviceStatuses()
}

// PruneProcessManagerStatus removes process-manager entries matching predicate.
func (sr *StatusReporter) PruneProcessManagerStatus(predicate func(uuid string, status *models.MicroserviceStatus) bool) {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	sr.statusReporterStatus.SetLastUpdate(time.Now().UnixMilli())
	sr.processManagerStatus.PruneMicroserviceStatus(predicate)
}

// UpdateLocalAPIStatus updates the local API status securely
func (sr *StatusReporter) UpdateLocalAPIStatus(fn func(*models.LocalAPIStatus)) {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	sr.statusReporterStatus.SetLastUpdate(time.Now().UnixMilli())
	fn(sr.localAPIStatus)
}

// UpdateSSHProxyManagerStatus updates the SSH proxy manager status securely
func (sr *StatusReporter) UpdateSSHProxyManagerStatus(fn func(*models.SSHProxyManagerStatus)) {
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
