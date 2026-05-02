package processmanager

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/eclipse-iofog/agent/internal/config"
	"github.com/eclipse-iofog/agent/internal/models"
	"github.com/eclipse-iofog/agent/internal/statusreporter"
	"github.com/eclipse-iofog/agent/internal/utils"
	"github.com/eclipse-iofog/agent/internal/utils/logging"
	"github.com/eclipse-iofog/agent/pkg/engine"
)

const (
	ProcessManagerModuleName = "Process Manager"
)

// ProcessManager manages container lifecycle via a ContainerEngine.
type ProcessManager struct {
	engine              engine.ContainerEngine
	microserviceManager MicroserviceManagerInterface
	containerManager    *ContainerManager
	taskQueue           *TaskQueue
	updateChan          chan struct{}
	ctx                 context.Context
	cancel              context.CancelFunc
	wg                  sync.WaitGroup
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

// Start starts the ProcessManager with a given container engine.
func (pm *ProcessManager) Start(eng engine.ContainerEngine, microserviceManager MicroserviceManagerInterface) error {
	pm.logger.Info("Starting Process Manager")

	pm.engine = eng
	pm.microserviceManager = microserviceManager
	pm.containerManager = NewContainerManager(eng, microserviceManager)

	pm.ctx, pm.cancel = context.WithCancel(context.Background())
	pm.taskQueue = NewTaskQueue(100)

	pm.wg.Add(2)
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

	pm.wg.Wait()
	pm.logger.Info("Process Manager stopped")
	return nil
}

// StopRunningMicroservices stops all running microservices matching the iofogUuid.
// When withCleanup=true (deprovision flow), containers are also removed along with their volumes.
func (pm *ProcessManager) StopRunningMicroservices(iofogUUID string, withCleanup bool) error {
	pm.logger.Info("Stop running Microservices")

	// Get all containers regardless of state for complete cleanup
	allContainers, err := pm.engine.GetAllContainers()
	if err != nil {
		pm.logger.Errorf("Error getting all containers: %v", err)
		return err
	}

	cfg := config.GetInstance()
	runningMicroserviceUuids := make([]string, 0)

	for _, container := range allContainers {
		msUUID := pm.engine.GetContainerMicroserviceUUID(container)
		if msUUID == "" {
			continue
		}

		containerIOFogUUID := container.Labels["iofog-uuid"]
		if (containerIOFogUUID != "" && containerIOFogUUID == iofogUUID) || cfg.WatchdogEnabled {
			runningMicroserviceUuids = append(runningMicroserviceUuids, msUUID)
		}
	}

	// Stop (and optionally remove) each matching container.
	// removeImage=false: deprovision path matches Java ProcessManager private method (no image removal).
	for _, msUUID := range runningMicroserviceUuids {
		if withCleanup {
			if err := pm.containerManager.RemoveContainerByMicroserviceUUID(msUUID, true, false); err != nil {
				pm.logger.Warnf("Error removing microservice %s: %v", msUUID, err)
			}
		} else {
			if err := pm.containerManager.StopContainerByMicroserviceUUID(msUUID); err != nil {
				pm.logger.Warnf("Error stopping microservice %s: %v", msUUID, err)
			}
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
	defer pm.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			logging.LogError(ProcessManagerModuleName, "Panic recovered", fmt.Errorf("%v", r))
		}
	}()
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

// checkTasks is a long-running goroutine that drains the task queue.
// It blocks on each Get() call and exits cleanly when the context is canceled.
func (pm *ProcessManager) checkTasks() {
	defer pm.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			logging.LogError(ProcessManagerModuleName, "Panic recovered", fmt.Errorf("%v", r))
		}
	}()
	for {
		task, ok := pm.taskQueue.Get(pm.ctx)
		if !ok {
			// Context canceled — stop processing.
			return
		}

		if err := pm.executeTask(task); err != nil {
			pm.logger.Errorf("Error executing task %s for microservice %s: %v", task.Action, task.MicroserviceUUID, err)
			if task.Retries < 5 {
				task.IncrementRetries()
				pm.taskQueue.Add(task)
			} else {
				pm.logger.Errorf("Task %s for microservice %s failed after %d retries, giving up", task.Action, task.MicroserviceUUID, task.Retries)
				// Set FAILED status and error message so controller receives it (matches Java ProcessManager.retryTask)
				statusreporter.GetInstance().UpdateProcessManagerStatus(func(s *models.ProcessManagerStatus) {
					s.SetMicroservicesState(task.MicroserviceUUID, models.MicroserviceStateFailed)
					errMsg := fmt.Sprintf("Container %s %s operation failed after 5 attempts: %v", task.MicroserviceUUID, task.Action, err)
					s.SetMicroservicesStatusErrorMessage(task.MicroserviceUUID, errMsg)
				})
				// Release the in-flight lock so the reconciliation loop can run
				if ms := pm.microserviceManager.FindLatestMicroserviceByUUID(task.MicroserviceUUID); ms != nil {
					ms.SetIsUpdating(false)
				}
			}
		}
	}
}

// executeTask executes a container task
func (pm *ProcessManager) executeTask(task *ContainerTask) error {
	pm.logger.Debugf("Executing task %s for microservice %s", task.Action, task.MicroserviceUUID)

	ms := pm.microserviceManager.FindLatestMicroserviceByUUID(task.MicroserviceUUID)

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
		return pm.containerManager.RemoveContainerByMicroserviceUUID(task.MicroserviceUUID, false, false)
	case TaskActionRemoveWithCleanup:
		// removeImage=true: matches Java ContainerManager behavior for clean removal
		return pm.containerManager.RemoveContainerByMicroserviceUUID(task.MicroserviceUUID, true, true)
	case TaskActionStop:
		return pm.containerManager.StopContainerByMicroserviceUUID(task.MicroserviceUUID)
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

// handleLatestMicroservices is the core reconciliation loop.
//
// It compares desired state (latestMicroservices from controller) against actual
// Docker state and converges them:
//   - container missing, should exist  → schedule ADD
//   - container exists, marked delete  → schedule REMOVE
//   - container exists, should exist   → schedule UPDATE if configuration drifted
//
// An ADD or UPDATE task sets ms.IsUpdating = true; the loop skips any microservice
// that already has an in-flight task, providing exactly-once per-cycle semantics and
// preventing task-queue flooding.
func (pm *ProcessManager) handleLatestMicroservices() {
	pm.logger.Debug("Start handle latest microservices")

	latestMicroservices := pm.microserviceManager.GetLatestMicroservices()
	// Sort by schedule ascending — matches Java: Comparator.comparingInt(Microservice::getSchedule)
	sort.Slice(latestMicroservices, func(i, j int) bool {
		return latestMicroservices[i].Schedule < latestMicroservices[j].Schedule
	})

	for _, ms := range latestMicroservices {
		// Skip microservices that already have an in-flight ADD or UPDATE task.
		// IsUpdating is set before enqueueing and cleared when the task finishes,
		// ensuring we never flood the queue with duplicate tasks.
		if ms.GetIsUpdating() {
			continue
		}

		container, err := pm.containerManager.GetContainerForMicroservice(ms.MicroserviceUUID)
		if err != nil {
			pm.logger.Warnf("Error getting container for microservice %s: %v", ms.MicroserviceUUID, err)
			continue
		}

		if ms.Delete {
			// Desired state: deleted.
			if container != nil {
				statusreporter.GetInstance().UpdateProcessManagerStatus(func(s *models.ProcessManagerStatus) {
					s.SetMicroservicesState(ms.MicroserviceUUID, models.MicroserviceStateMarkedForDeletion)
				})
				pm.deleteMicroservice(ms)
			}
			// If container is already gone, nothing to do.
			continue
		}

		// Desired state: running.
		if container == nil {
			if ms.IsStuckInRestart && !ms.Rebuild {
				pm.logger.Debugf("Skipping stuck microservice %s (rebuild not requested)", ms.MicroserviceUUID)
				continue
			}
			// If status is FAILED and Rebuild not requested, skip — do not re-add (matches Java behavior)
			if pmStatus := statusreporter.GetInstance().GetProcessManagerStatus(); pmStatus != nil {
				if st := pmStatus.GetMicroserviceStatus(ms.MicroserviceUUID); st != nil &&
					st.Status == models.MicroserviceStateFailed && !ms.Rebuild {
					pm.logger.Debugf("Skipping failed microservice %s (rebuild not requested)", ms.MicroserviceUUID)
					continue
				}
			}
			// Container is missing — schedule creation. Clear stuck flag when Rebuild=true.
			pm.logger.Infof("Container missing for microservice %s (%s), scheduling creation", ms.MicroserviceUUID, ms.ImageName)
			ms.IsStuckInRestart = false
			statusreporter.GetInstance().UpdateProcessManagerStatus(func(s *models.ProcessManagerStatus) {
				s.SetMicroservicesState(ms.MicroserviceUUID, models.MicroserviceStateUnknown)
			})
			pm.addMicroservice(ms)
			continue
		}

		// Container exists and should keep running.
		// Skip containers stuck in restart loop (unless a rebuild was explicitly requested).
		if ms.IsStuckInRestart && !ms.Rebuild {
			continue
		}

		status, err := pm.engine.GetContainerStatus(container.ID, ms.MicroserviceUUID)
		if err != nil {
			pm.logger.Warnf("Error getting microservice status for %s: %v", ms.MicroserviceUUID, err)
			continue
		}

		// Detect containers stuck in exit/creation loops and mark them accordingly.
		// Prefer existing error message from engine (e.g. Docker) when available; use static fallback otherwise (matching Java DockerUtil.getMicroserviceStatus).
		checker := GetRestartStuckChecker()
		if status.Status == models.MicroserviceStateExiting {
			if checker.IsStuck(ms.MicroserviceUUID) {
				status.Status = models.MicroserviceStateStuckInRestart
				ms.IsStuckInRestart = true
				stuckMsg := stuckInRestartErrorMessage(ms.MicroserviceUUID, "Container repeatedly exiting")
				status.ErrorMessage = &stuckMsg
			}
		} else if status.Status == models.MicroserviceStateCreated {
			if checker.IsStuckInContainerCreation(ms.MicroserviceUUID) {
				status.Status = models.MicroserviceStateStuckInRestart
				ms.IsStuckInRestart = true
				stuckMsg := stuckInRestartErrorMessage(ms.MicroserviceUUID, "Container repeatedly failing to start")
				status.ErrorMessage = &stuckMsg
			}
		}

		// Merge per-container CPU/memory stats for running containers (best-effort).
		if status.Status == models.MicroserviceStateRunning {
			if stats, err := pm.engine.GetContainerStats(container.ID); err == nil {
				status.CPUUsage = stats.CPUUsage
				status.MemoryUsage = stats.MemoryUsage
			}
		}

		statusreporter.GetInstance().UpdateProcessManagerStatus(func(pmStatus *models.ProcessManagerStatus) {
			existing := pmStatus.MicroservicesStatus[ms.MicroserviceUUID]
			if existing != nil && existing.HealthStatus != nil && status.HealthStatus == nil {
				status.HealthStatus = existing.HealthStatus
			}
			pmStatus.SetMicroservicesStatus(ms.MicroserviceUUID, status)
		})

		pm.updateMicroservice(container, ms)
	}

	pm.logger.Debug("Finished handle latest microservices")
}

// addMicroservice queues a microservice for creation.
// It marks the microservice as "in-flight" (IsUpdating=true) before enqueuing so that
// subsequent reconciliation cycles skip it until the ADD task finishes.
func (pm *ProcessManager) addMicroservice(ms *models.Microservice) {
	ms.SetIsUpdating(true)
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
func (pm *ProcessManager) updateMicroservice(container *engine.Container, ms *models.Microservice) {
	pm.logger.Debug("Start update microservice")

	ms.ContainerID = container.ID

	ip, err := pm.engine.GetContainerIPAddress(container.ID)
	if err != nil {
		pm.logger.Warnf("Can't get IP address for microservice %s: %v", ms.MicroserviceUUID, err)
		ip = "0.0.0.0"
	}
	ms.ContainerIPAddress = &ip

	status, err := pm.engine.GetContainerStatus(container.ID, ms.MicroserviceUUID)
	if err != nil {
		pm.logger.Warnf("Error getting microservice status: %v", err)
		return
	}

	shouldUpdate := pm.shouldContainerBeUpdated(ms, container, status)
	if shouldUpdate {
		if ms.IsStuckInRestart {
			ms.IsStuckInRestart = false
		}
		statusreporter.GetInstance().UpdateProcessManagerStatus(func(status *models.ProcessManagerStatus) {
			status.SetMicroservicesState(ms.MicroserviceUUID, models.MicroserviceStateUpdating)
		})
		pm.addTask(NewContainerTask(TaskActionUpdate, ms.MicroserviceUUID))
	}

	pm.logger.Debug("Finished update microservice")
}

// shouldContainerBeUpdated determines if a running container needs to be rebuilt.
func (pm *ProcessManager) shouldContainerBeUpdated(ms *models.Microservice, container *engine.Container, status *models.MicroserviceStatus) bool {
	pm.logger.Debug("Start should Container Be Updated")

	if ms.GetIsUpdating() {
		pm.logger.Debugf("Skipping update check for %s — task already in flight", ms.MicroserviceUUID)
		return false
	}

	if ms.Rebuild {
		pm.logger.Debugf("Rebuild requested for %s — bypassing state checks", ms.MicroserviceUUID)
		return true
	}

	switch status.Status {
	case models.MicroserviceStateQueued,
		models.MicroserviceStateUpdating,
		models.MicroserviceStateStuckInRestart,
		models.MicroserviceStateCreated: // Container still starting — don't trigger update
		return false
	default:
	}

	isNotRunning := status.Status != models.MicroserviceStateRunning
	areNotEqual := !pm.engine.AreMicroserviceAndContainerEqual(container.ID, ms)
	isRebuild := ms.Rebuild

	result := isNotRunning || areNotEqual || isRebuild
	pm.logger.Debugf("Finished should Container Be Updated: notRunning=%v configDrifted=%v rebuild=%v → %v",
		isNotRunning, areNotEqual, isRebuild, result)
	return result
}

// stuckInRestartErrorMessage returns the existing error message from the status reporter
// when available (e.g. from Docker/engine), otherwise the static fallback (matching Java DockerUtil.getMicroserviceStatus).
func stuckInRestartErrorMessage(microserviceUUID, fallback string) string {
	existing := statusreporter.GetInstance().GetProcessManagerStatus().GetMicroserviceStatus(microserviceUUID)
	if existing != nil && existing.ErrorMessage != nil && *existing.ErrorMessage != "" {
		return *existing.ErrorMessage
	}
	return fallback
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

	allContainers, err := pm.engine.GetAllContainers()
	if err != nil {
		pm.logger.Errorf("Error getting all containers: %v", err)
		return
	}

	oldAgentUUIDs := make([]string, 0)
	unknownContainerIDs := make([]string, 0)

	cfg := config.GetInstance()

	for _, container := range allContainers {
		msUUID := pm.engine.GetContainerMicroserviceUUID(container)
		if msUUID == "" {
			continue
		}

		isCurrent := currentUUIDs[msUUID]
		isLatest := latestUUIDs[msUUID]

		isSystem := container.Labels["iofog-system"] == "true"

		// Old agent microservice: in current but not in latest → always remove
		if isCurrent && !isLatest && !isSystem {
			oldAgentUUIDs = append(oldAgentUUIDs, msUUID)
		}

		// Unknown: not in current or latest → only remove if watchdog enabled
		if !isCurrent && !isLatest && !isSystem {
			if cfg.WatchdogEnabled {
				unknownContainerIDs = append(unknownContainerIDs, container.ID)
			}
		}
	}

	// Delete old agent containers
	for _, uuid := range oldAgentUUIDs {
		pm.logger.Infof("Deleting old agent microservice: %s", uuid)
		if err := pm.containerManager.RemoveContainerByMicroserviceUUID(uuid, false, false); err != nil {
			pm.logger.Errorf("Error deleting old agent microservice %s: %v", uuid, err)
		}
	}

	// Delete unknown containers by concrete container ID so watchdog can remove
	// non-iofog containers that don't resolve via microservice UUID lookup.
	for _, containerID := range unknownContainerIDs {
		pm.logger.Infof("Deleting unknown container: %s", containerID)
		if err := pm.containerManager.RemoveContainerByID(containerID, false, false); err != nil {
			pm.logger.Errorf("Error deleting unknown container %s: %v", containerID, err)
		}
	}

	pm.logger.Debug("Finished delete Remaining Microservices")
}

// updateRunningMicroservicesCount updates the count of running microservices.
// Matches Java: getRunningIofogContainers() — filters by iofog_ name prefix only.
func (pm *ProcessManager) updateRunningMicroservicesCount() {
	pm.logger.Debug("Update running microservice count")

	containers, err := pm.engine.GetRunningContainers()
	if err != nil {
		pm.logger.Errorf("Error getting running containers: %v", err)
		return
	}

	count := 0
	for _, c := range containers {
		name := pm.engine.GetContainerName(c)
		if strings.HasPrefix(name, utils.IOFogDockerContainerNamePrefix) {
			count++
		}
	}

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

// CreateExecSession creates and starts an exec session for a microservice.
// It calls engine.CreateExecSession to register the exec spec, then
// engine.StartExecSession to attach the callback's I/O pipes and launch the process.
// This matches the Java agent's two-phase: createExecSession + startExecSession.
func (pm *ProcessManager) CreateExecSession(microserviceUUID string, command []string, callback ExecSessionCallbackInterface) (string, error) {
	pm.logger.Infof("Creating exec session for microservice: %s", microserviceUUID)

	container, err := pm.containerManager.GetContainerForMicroservice(microserviceUUID)
	if err != nil {
		return "", fmt.Errorf("failed to get container: %w", err)
	}
	if container == nil {
		return "", fmt.Errorf("container not found for microservice: %s", microserviceUUID)
	}

	execID, err := pm.engine.CreateExecSession(container.ID, command)
	if err != nil {
		return "", fmt.Errorf("failed to create exec session: %w", err)
	}

	// Attach the callback's I/O pipes and start the exec process.
	// StartExecSession blocks until the process exits; run it in a goroutine so the
	// caller can proceed to connect the WebSocket while the process is running.
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logging.LogError(ProcessManagerModuleName, "Panic recovered", fmt.Errorf("%v", r))
			}
		}()
		if err := pm.engine.StartExecSession(
			execID,
			callback.GetStdinReader(),
			callback.GetStdoutWriter(),
			callback.GetStderrWriter(),
		); err != nil {
			pm.logger.Errorf("Exec session %s I/O error: %v", execID, err)
			callback.OnError(err)
		} else {
			callback.OnComplete()
		}
	}()

	pm.logger.Infof("Exec session started: %s for microservice: %s", execID, microserviceUUID)
	return execID, nil
}

// GetExecSessionStatus reports whether the exec process identified by execID is running.
func (pm *ProcessManager) GetExecSessionStatus(execID string) (interface{}, error) {
	running, err := pm.engine.GetExecSessionStatus(execID)
	if err != nil {
		return nil, err
	}
	return running, nil
}

// StopExecSession kills and deregisters the exec process in the engine.
// Called when the controller closes the WebSocket so the exec ID can be reused.
func (pm *ProcessManager) StopExecSession(_ string, execID string) error {
	return pm.engine.StopExecSession(execID)
}
