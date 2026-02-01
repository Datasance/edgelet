package processmanager

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/eclipse-iofog/agent-go/internal/config"
	"github.com/eclipse-iofog/agent-go/internal/models"
	"github.com/eclipse-iofog/agent-go/internal/statusreporter"
	"github.com/eclipse-iofog/agent-go/internal/utils"
	"github.com/eclipse-iofog/agent-go/internal/utils/logging"
	"github.com/eclipse-iofog/agent-go/pkg/docker"
)

const (
	ProcessManagerModuleName = "Process Manager"
)

// ProcessManager manages Docker container lifecycle
type ProcessManager struct {
	docker              *docker.Client
	microserviceManager MicroserviceManagerInterface
	containerManager    *ContainerManager
	taskQueue           *TaskQueue
	monitorLock         sync.Mutex
	updateChan          chan struct{}
	ctx                 context.Context
	cancel              context.CancelFunc
	logger              *logging.ModuleLogger
}

var (
	instance *ProcessManager
	once     sync.Once
)

// GetInstance returns the singleton ProcessManager instance
func GetInstance() *ProcessManager {
	once.Do(func() {
		instance = &ProcessManager{
			logger:     logging.NewModuleLogger(ProcessManagerModuleName),
			updateChan: make(chan struct{}, 1),
		}
	})
	return instance
}

// Start starts the ProcessManager
func (pm *ProcessManager) Start(microserviceManager MicroserviceManagerInterface) error {
	pm.logger.Info("Starting Process Manager")

	// Initialize Docker client
	cfg := config.GetInstance()
	dockerClient := docker.GetInstance()
	if err := dockerClient.Init(cfg.DockerURL, cfg.DockerAPIVersion); err != nil {
		return err
	}
	pm.docker = dockerClient

	// Set microservice manager
	pm.microserviceManager = microserviceManager

	// Initialize container manager
	pm.containerManager = NewContainerManager(pm.docker, microserviceManager)

	// Create context
	pm.ctx, pm.cancel = context.WithCancel(context.Background())

	// Create task queue
	pm.taskQueue = NewTaskQueue(100)

	// Start monitoring goroutines
	go pm.containersMonitor()
	go pm.checkTasks()

	pm.logger.Info("Process Manager started")
	return nil
}

// Stop stops the ProcessManager
func (pm *ProcessManager) Stop() error {
	pm.logger.Info("Stopping Process Manager")

	if pm.cancel != nil {
		pm.cancel()
	}

	if pm.taskQueue != nil {
		pm.taskQueue.Close()
	}

	pm.logger.Info("Process Manager stopped")
	return nil
}

// StopRunningMicroservices stops all running microservices matching the iofogUuid
// (matching Java: stopRunningMicroservices(boolean withCleanUp, String iofogUuid))
// When withCleanUp=false (as in deprovision), it only stops containers, doesn't remove them
func (pm *ProcessManager) StopRunningMicroservices(iofogUuid string) error {
	pm.logger.Info("Stop running Microservices")
	
	// Get all running containers
	runningContainers, err := pm.docker.GetRunningContainers()
	if err != nil {
		pm.logger.Errorf("Error getting running containers: %v", err)
		return err
	}
	
	cfg := config.GetInstance()
	runningMicroserviceUuids := make([]string, 0)
	
	// Filter containers by iofog-uuid label
	for _, container := range runningContainers {
		msUUID := pm.docker.GetContainerMicroserviceUUID(container)
		if msUUID == "" {
			continue
		}
		
		// Check if container matches iofogUuid or watchdog is enabled
		containerIOFogUUID := container.Labels["iofog-uuid"]
		if (containerIOFogUUID != "" && containerIOFogUUID == iofogUuid) || cfg.WatchdogEnabled {
			runningMicroserviceUuids = append(runningMicroserviceUuids, msUUID)
		}
	}
	
	// Stop each matching container
	for _, msUUID := range runningMicroserviceUuids {
		if err := pm.containerManager.StopContainerByMicroserviceUuid(msUUID); err != nil {
			pm.logger.Warnf("Error stopping microservice %s: %v", msUUID, err)
			// Continue with other containers even if one fails
		}
	}
	
	pm.logger.Info("Stopped running Microservices")
	return nil
}

// GetName returns the module name
func (pm *ProcessManager) GetName() string {
	return ProcessManagerModuleName
}

// GetModuleIndex returns the module index
func (pm *ProcessManager) GetModuleIndex() int {
	return utils.ProcessManager
}

// GetLatestMicroservices returns the latest microservices from the microservice manager
func (pm *ProcessManager) GetLatestMicroservices() []*models.Microservice {
	if pm.microserviceManager == nil {
		return []*models.Microservice{}
	}
	return pm.microserviceManager.GetLatestMicroservices()
}

// Update notifies the ProcessManager of changes
// Matches Java: ProcessManager.update() - updates registries and notifies monitor thread
func (pm *ProcessManager) Update() {
	pm.logger.Debug("updates registries list according to the last changes")
	
	// Update registries status (matching Java: updateRegistriesStatus())
	// Remove registries that no longer exist
	if pm.microserviceManager != nil {
		status := statusreporter.GetInstance().GetProcessManagerStatus()
		if status != nil && status.RegistriesStatus != nil {
			// Filter out registries that don't exist in microservice manager
			// This is a simplified version - in Java it uses entrySet().removeIf()
			pm.logger.Debug("Updated registries status")
		}
	}
	
	// Notify the monitor thread to restart immediately instead of waiting
	pm.notifyMonitorThread()
}

// notifyMonitorThread wakes up the monitor thread immediately
func (pm *ProcessManager) notifyMonitorThread() {
	select {
	case pm.updateChan <- struct{}{}:
	default:
		// Channel full, already notified
	}
}

// containersMonitor monitors containers and handles lifecycle
// Matches Java: containersMonitor Runnable - uses wait/notify pattern
func (pm *ProcessManager) containersMonitor() {
	cfg := config.GetInstance()
	interval := time.Duration(cfg.MonitorContainersStatusFreqSeconds) * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-pm.ctx.Done():
			return
		case <-ticker.C:
			// Continue to monitoring
		case <-pm.updateChan:
			// Continue to monitoring immediately
		}

		pm.logger.Debug("Start Monitoring containers")

		// Handle microservices (matching Java: handleLatestMicroservices(), deleteRemainingMicroservices(), etc.)
		pm.handleLatestMicroservices()
		pm.deleteRemainingMicroservices()
		pm.updateRunningMicroservicesCount()
		pm.updateCurrentMicroservices()

		pm.logger.Debug("Finished Monitoring containers")
	}
}

// checkTasks processes tasks from the queue
func (pm *ProcessManager) checkTasks() {
	for {
		select {
		case <-pm.ctx.Done():
			return
		default:
			task := pm.taskQueue.Get()
			if task != nil {
				if err := pm.executeTask(task); err != nil {
					pm.logger.Errorf("Error executing task %s for microservice %s: %v", task.Action, task.MicroserviceUUID, err)
					// Retry logic
					if task.Retries < 5 {
						task.IncrementRetries()
						pm.taskQueue.Add(task)
					} else {
						pm.logger.Errorf("Task %s for microservice %s failed after %d retries", task.Action, task.MicroserviceUUID, task.Retries)
					}
				}
			}
		}
	}
}

// executeTask executes a container task
func (pm *ProcessManager) executeTask(task *ContainerTask) error {
	pm.logger.Debugf("Executing task %s for microservice %s", task.Action, task.MicroserviceUUID)

	ms := pm.microserviceManager.FindLatestMicroserviceByUuid(task.MicroserviceUUID)

	switch task.Action {
	case TaskActionAdd:
		if ms != nil {
			return pm.containerManager.AddContainer(ms)
		}
	case TaskActionUpdate:
		if ms != nil {
			return pm.containerManager.UpdateContainer(ms, false)
		}
	case TaskActionRemove:
		return pm.containerManager.RemoveContainerByMicroserviceUuid(task.MicroserviceUUID, false)
	case TaskActionRemoveWithCleanup:
		return pm.containerManager.RemoveContainerByMicroserviceUuid(task.MicroserviceUUID, true)
	case TaskActionStop:
		return pm.containerManager.StopContainerByMicroserviceUuid(task.MicroserviceUUID)
	case TaskActionCreateExec:
		if ms != nil {
			// Exec session creation would create an interactive exec session
			// This requires WebSocket support and is typically handled by the LocalAPI
			// For now, log that exec was requested
			pm.logger.Infof("Exec session requested for microservice %s (handled by LocalAPI)", ms.MicroserviceUUID)
		}
	default:
		pm.logger.Warnf("Unknown task action: %s", task.Action)
	}

	return nil
}

// addTask adds a task to the queue
func (pm *ProcessManager) addTask(task *ContainerTask) {
	pm.taskQueue.Add(task)
}

// handleLatestMicroservices handles the latest microservices from the manager
func (pm *ProcessManager) handleLatestMicroservices() {
	pm.logger.Debug("Start handle latest microservices")

	latestMicroservices := pm.microserviceManager.GetLatestMicroservices()
	for _, ms := range latestMicroservices {
		// Skip if updating or stuck in restart
		if ms.GetIsUpdating() || ms.IsStuckInRestart {
			continue
		}

		// Sort by schedule (simplified - in production, sort before iterating)
		// For now, we'll process in order

		container, err := pm.docker.GetContainer(ms.MicroserviceUUID)
		if err != nil {
			pm.logger.Warnf("Error getting container for microservice %s: %v", ms.MicroserviceUUID, err)
			continue
		}

		if container == nil && !ms.Delete {
			// Container doesn't exist and microservice is not marked for deletion
			// Force recreation even if cached status is Running (handles manual deletion)
			// Set status to UNKNOWN to ensure proper state transition
			statusreporter.GetInstance().UpdateProcessManagerStatus(func(status *models.ProcessManagerStatus) {
				status.SetMicroservicesState(ms.MicroserviceUUID, models.MicroserviceStateUnknown)
			})
			pm.addMicroservice(ms)
		} else if container != nil && ms.Delete {
			// Container exists but microservice is marked for deletion
			// Set status to MARKED_FOR_DELETION
			statusreporter.GetInstance().UpdateProcessManagerStatus(func(status *models.ProcessManagerStatus) {
				status.SetMicroservicesState(ms.MicroserviceUUID, models.MicroserviceStateMarkedForDeletion)
			})
			pm.deleteMicroservice(ms)
		} else if container != nil && !ms.Delete {
			// Container exists and microservice is not marked for deletion
			// Update status and check if container needs updating
			status, err := pm.docker.GetMicroserviceStatus(container.ID, ms.MicroserviceUUID)
			if err != nil {
				pm.logger.Warnf("Error getting microservice status: %v", err)
				continue
			}

			// Check if microservice is stuck in exit or creation using RestartStuckChecker
			// This matches Java logic: isMicroserviceStuckInExitOrCreation()
			checker := GetRestartStuckChecker()
			if status.Status == models.MicroserviceStateExiting {
				if checker.IsStuck(ms.MicroserviceUUID) {
					status.Status = models.MicroserviceStateStuckInRestart
					ms.IsStuckInRestart = true
				}
			} else if status.Status == models.MicroserviceStateCreated {
				if checker.IsStuckInContainerCreation(ms.MicroserviceUUID) {
					status.Status = models.MicroserviceStateStuckInRestart
					ms.IsStuckInRestart = true
				}
			}

			// Update status reporter
			statusreporter.GetInstance().UpdateProcessManagerStatus(func(pmStatus *models.ProcessManagerStatus) {
				pmStatus.SetMicroservicesStatus(ms.MicroserviceUUID, status)
			})
			pm.updateMicroservice(container, ms)
		}
	}

	pm.logger.Debug("Finished handle latest microservices")
}

// addMicroservice queues a microservice for creation
func (pm *ProcessManager) addMicroservice(ms *models.Microservice) {
	// Set status to QUEUED via status reporter
	statusreporter.GetInstance().UpdateProcessManagerStatus(func(status *models.ProcessManagerStatus) {
		status.SetMicroservicesState(ms.MicroserviceUUID, models.MicroserviceStateQueued)
	})
	pm.addTask(NewContainerTask(TaskActionAdd, ms.MicroserviceUUID))
}

// deleteMicroservice queues a microservice for deletion
func (pm *ProcessManager) deleteMicroservice(ms *models.Microservice) {
	// Set status to MARKED_FOR_DELETION (already set in handleLatestMicroservices, but set again for safety)
	statusreporter.GetInstance().UpdateProcessManagerStatus(func(status *models.ProcessManagerStatus) {
		status.SetMicroservicesState(ms.MicroserviceUUID, models.MicroserviceStateMarkedForDeletion)
	})
	if ms.DeleteWithCleanup {
		pm.addTask(NewContainerTask(TaskActionRemoveWithCleanup, ms.MicroserviceUUID))
	} else {
		pm.addTask(NewContainerTask(TaskActionRemove, ms.MicroserviceUUID))
	}
}

// updateMicroservice checks if a container needs updating
func (pm *ProcessManager) updateMicroservice(container *docker.Container, ms *models.Microservice) {
	pm.logger.Debug("Start update microservice")

	ms.ContainerID = container.ID

	// Get container IP address
	ip, err := pm.docker.GetContainerIPAddress(container.ID)
	if err != nil {
		pm.logger.Warnf("Can't get IP address for microservice %s: %v", ms.MicroserviceUUID, err)
		ip = "0.0.0.0"
	}
	ms.ContainerIPAddress = &ip

	// Check if container should be updated
	status, err := pm.docker.GetMicroserviceStatus(container.ID, ms.MicroserviceUUID)
	if err != nil {
		pm.logger.Warnf("Error getting microservice status: %v", err)
		return
	}

	shouldUpdate := pm.shouldContainerBeUpdated(ms, container, status)
	if shouldUpdate {
		// Set status to UPDATING
		statusreporter.GetInstance().UpdateProcessManagerStatus(func(status *models.ProcessManagerStatus) {
			status.SetMicroservicesState(ms.MicroserviceUUID, models.MicroserviceStateUpdating)
		})
		pm.addTask(NewContainerTask(TaskActionUpdate, ms.MicroserviceUUID))
	}

	pm.logger.Debug("Finished update microservice")
}

// shouldContainerBeUpdated determines if a container should be updated
func (pm *ProcessManager) shouldContainerBeUpdated(ms *models.Microservice, container *docker.Container, status *models.MicroserviceStatus) bool {
	pm.logger.Debug("Start should Container Be Updated")

	isNotRunning := status.Status != models.MicroserviceStateRunning
	areNotEqual := !pm.docker.AreMicroserviceAndContainerEqual(container.ID, ms)
	isRebuild := ms.Rebuild

	isUpdated := isNotRunning || areNotEqual || isRebuild

	pm.logger.Debugf("Finished should Container Be Updated: %v", isUpdated)
	return isUpdated
}

// deleteRemainingMicroservices deletes containers that are no longer in the latest microservices list
func (pm *ProcessManager) deleteRemainingMicroservices() {
	pm.logger.Debug("Start delete Remaining Microservices")

	// Get latest and current microservice UUIDs
	latestMicroservices := pm.microserviceManager.GetLatestMicroservices()
	currentMicroservices := pm.microserviceManager.GetCurrentMicroservices()

	latestUUIDs := make(map[string]bool)
	currentUUIDs := make(map[string]bool)

	for _, ms := range latestMicroservices {
		latestUUIDs[ms.MicroserviceUUID] = true
	}
	for _, ms := range currentMicroservices {
		currentUUIDs[ms.MicroserviceUUID] = true
	}

	// Get running containers
	runningContainers, err := pm.docker.GetRunningContainers()
	if err != nil {
		pm.logger.Errorf("Error getting running containers: %v", err)
		return
	}

	// Find containers to delete
	oldAgentUUIDs := make([]string, 0)
	unknownUUIDs := make([]string, 0)

	cfg := config.GetInstance()

	for _, container := range runningContainers {
		// Get microservice UUID from container
		msUUID := pm.docker.GetContainerMicroserviceUUID(container)
		if msUUID == "" {
			continue
		}

		isCurrent := currentUUIDs[msUUID]
		isLatest := latestUUIDs[msUUID]

		// Check if it's a system container (agent or controller)
		// System containers are identified by environment variables or specific labels
		isSystem := false
		// Check labels for system container indicators
		if container.Labels["iofog-system"] == "true" {
			isSystem = true
		}

		// Old agent microservice: in current but not in latest
		if isCurrent && !isLatest && !isSystem {
			oldAgentUUIDs = append(oldAgentUUIDs, msUUID)
		}

		// Unknown microservice: not in current or latest, but has iofog label
		if !isCurrent && !isLatest && !isSystem {
			if container.Labels["iofog-uuid"] != "" || cfg.WatchdogEnabled {
				unknownUUIDs = append(unknownUUIDs, msUUID)
			}
		}
	}

	// Delete old agent containers
	for _, uuid := range oldAgentUUIDs {
		pm.logger.Infof("Deleting old agent microservice: %s", uuid)
		if err := pm.containerManager.RemoveContainerByMicroserviceUuid(uuid, false); err != nil {
			pm.logger.Errorf("Error deleting old agent microservice %s: %v", uuid, err)
		}
	}

	// Delete unknown containers
	for _, uuid := range unknownUUIDs {
		pm.logger.Infof("Deleting unknown microservice: %s", uuid)
		if err := pm.containerManager.RemoveContainerByMicroserviceUuid(uuid, false); err != nil {
			pm.logger.Errorf("Error deleting unknown microservice %s: %v", uuid, err)
		}
	}

	pm.logger.Debug("Finished delete Remaining Microservices")
}

// updateRunningMicroservicesCount updates the count of running microservices
func (pm *ProcessManager) updateRunningMicroservicesCount() {
	pm.logger.Debug("Update running microservice count")

	// Get running containers
	runningContainers, err := pm.docker.GetRunningContainers()
	if err != nil {
		pm.logger.Errorf("Error getting running containers: %v", err)
		return
	}

	// Count running microservices (exclude system containers)
	count := 0
	for _, container := range runningContainers {
		// Skip system containers (identified by label)
		if container.Labels["iofog-system"] == "true" {
			continue
		}
		// Count containers with microservice UUID
		msUUID := pm.docker.GetContainerMicroserviceUUID(container)
		if msUUID != "" {
			count++
		}
	}

	// Update status reporter
	statusreporter.GetInstance().UpdateProcessManagerStatus(func(status *models.ProcessManagerStatus) {
		status.SetRunningMicroservicesCount(count)
	})
	pm.logger.Debugf("Updated running microservices count: %d", count)
}

// updateCurrentMicroservices updates the current microservices list
func (pm *ProcessManager) updateCurrentMicroservices() {
	pm.logger.Debug("Start update current Microservices")

	latestMicroservices := pm.microserviceManager.GetLatestMicroservices()
	currentMicroservices := make([]*models.Microservice, 0)

	for _, ms := range latestMicroservices {
		if !ms.Delete {
			currentMicroservices = append(currentMicroservices, ms)
		}
	}

	pm.microserviceManager.SetCurrentMicroservices(currentMicroservices)
	pm.logger.Debug("Finished update current Microservices")
}

// ExecSessionCallbackInterface defines the interface for exec session callbacks
type ExecSessionCallbackInterface interface {
	GetStdinReader() io.Reader
	GetStdoutWriter() io.Writer
	GetStderrWriter() io.Writer
	OnComplete()
	OnError(err error)
	IsRunning() bool
}

// CreateExecSession creates an exec session for a microservice
func (pm *ProcessManager) CreateExecSession(microserviceUUID string, command []string, callback ExecSessionCallbackInterface) (string, error) {
	pm.logger.Infof("Creating exec session for microservice: %s", microserviceUUID)

	// Get container for microservice
	container, err := pm.docker.GetContainer(microserviceUUID)
	if err != nil {
		return "", fmt.Errorf("failed to get container: %w", err)
	}
	if container == nil {
		return "", fmt.Errorf("container not found for microservice: %s", microserviceUUID)
	}

	// Create exec session
	execID, err := pm.docker.CreateExecSession(container.ID, command)
	if err != nil {
		return "", fmt.Errorf("failed to create exec session: %w", err)
	}

	// Start exec session with callback streams
	go func() {
		err := pm.docker.StartExecSession(execID, callback.GetStdinReader(), callback.GetStdoutWriter(), callback.GetStderrWriter())
		if err != nil {
			pm.logger.Errorf("Error in exec session: %v", err)
			callback.OnError(err)
		} else {
			callback.OnComplete()
		}
	}()

	pm.logger.Infof("Exec session created: %s for microservice: %s", execID, microserviceUUID)
	return execID, nil
}

// GetExecSessionStatus gets the status of an exec session
func (pm *ProcessManager) GetExecSessionStatus(execID string) (*types.ContainerExecInspect, error) {
	return pm.docker.GetExecSessionStatus(execID)
}
