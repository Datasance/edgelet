package processmanager

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/eclipse-iofog/agent/internal/config"
	"github.com/eclipse-iofog/agent/internal/models"
	"github.com/eclipse-iofog/agent/internal/network"
	"github.com/eclipse-iofog/agent/internal/statusreporter"
	"github.com/eclipse-iofog/agent/internal/store"
	"github.com/eclipse-iofog/agent/internal/utils"
	"github.com/eclipse-iofog/agent/internal/utils/logging"
	"github.com/eclipse-iofog/agent/internal/workloadmeta"
	"github.com/eclipse-iofog/agent/pkg/engine"
	"github.com/eclipse-iofog/agent/pkg/imageref"
	"gopkg.in/yaml.v3"
)

const (
	ProcessManagerModuleName    = "Process Manager"
	defaultShutdownDrainTimeout = 45 * time.Second
	shutdownDrainPollInterval   = 1 * time.Second
	localReconcileMaxFailures   = 5
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

// LocalDeployProgressCallback reports local deployment runtime stage transitions.
type LocalDeployProgressCallback func(stage string, message string)

func emitLocalDeployProgress(cb LocalDeployProgressCallback, stage, message string) {
	if cb != nil {
		cb(stage, message)
	}
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

// DrainRuntimeForShutdown best-effort drains runtime tasks before manager teardown.
// It is used during daemon shutdown so containerd can stop without lingering shims.
func (pm *ProcessManager) DrainRuntimeForShutdown(timeout time.Duration) error {
	if pm.engine == nil {
		return nil
	}
	if timeout <= 0 {
		timeout = defaultShutdownDrainTimeout
	}

	deadline := time.Now().Add(timeout)
	for {
		running, err := pm.engine.GetRunningContainers()
		if err != nil {
			return fmt.Errorf("list running containers during shutdown drain: %w", err)
		}

		runtimeIDs := make([]string, 0, len(running))
		for _, container := range running {
			if msUUID := pm.engine.GetContainerMicroserviceUUID(container); msUUID != "" {
				runtimeIDs = append(runtimeIDs, container.ID)
			}
		}
		if len(runtimeIDs) == 0 {
			pm.logger.Info("Shutdown runtime drain complete: no running workload containers")
			return nil
		}

		for _, containerID := range runtimeIDs {
			if err := pm.engine.StopContainer(containerID); err != nil {
				pm.logger.Warnf("Shutdown drain: graceful stop failed for container %s: %v", containerID, err)
				if killErr := pm.engine.KillContainer(containerID); killErr != nil {
					pm.logger.Warnf("Shutdown drain: force stop failed for container %s: %v", containerID, killErr)
				}
			}
		}

		if time.Now().After(deadline) {
			return fmt.Errorf(
				"timed out draining runtime containers after %s; remaining container IDs: %s",
				timeout,
				strings.Join(runtimeIDs, ","),
			)
		}
		time.Sleep(shutdownDrainPollInterval)
	}
}

// StopRunningMicroservices stops all running microservices matching the iofogUuid.
// When withCleanup=true (deprovision flow), containers are also removed along with their volumes.
func (pm *ProcessManager) StopRunningMicroservices(iofogUUID string, withCleanup bool) error {
	return pm.StopRunningMicroservicesWithScope(iofogUUID, withCleanup, true)
}

// StopRunningMicroservicesWithScope stops running microservices matching iofogUuid.
// includeLocal=false preserves local-scope workloads during deprovision cleanup.
func (pm *ProcessManager) StopRunningMicroservicesWithScope(iofogUUID string, withCleanup bool, includeLocal bool) error {
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
		msUUID := workloadmeta.MicroserviceUIDFromLabels(container.Labels)
		if msUUID == "" {
			continue
		}

		if !workloadmeta.IsManagedByIofog(container.Labels) {
			continue
		}
		if !includeLocal && strings.EqualFold(strings.TrimSpace(container.Labels[workloadmeta.LabelScope]), workloadmeta.ScopeLocal) {
			continue
		}
		containerIOFogUUID := strings.TrimSpace(container.Labels[workloadmeta.LabelNodeUID])
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
		pm.reconcileLocalDeployments()
		pm.deleteRemainingMicroservices()
		pm.updateRunningMicroservicesCount()
		pm.updateCurrentMicroservices()

		pm.logger.Debug("Finished Monitoring containers")
	}
}

func (pm *ProcessManager) reconcileLocalDeployments() {
	items, err := store.GetInstance().ListLocalDeployedMicroservices()
	if err != nil {
		pm.logger.Warnf("local reconcile list deployments failed: %v", err)
		return
	}
	for _, item := range items {
		if item == nil || strings.TrimSpace(item.LocalUUID) == "" {
			continue
		}
		pm.reconcileOneLocalDeployment(item)
	}
}

func (pm *ProcessManager) reconcileOneLocalDeployment(item *models.LocalDeployedMicroservice) {
	now := time.Now().Unix()
	item.NormalizeDefaults()
	item.LastReconcileAt = now

	desired := strings.ToLower(strings.TrimSpace(item.DesiredState))
	if desired == "" {
		desired = "running"
	}

	container, err := pm.containerManager.GetContainerForMicroservice(item.LocalUUID)
	if err != nil {
		item.LastError = err.Error()
		item.RuntimeState = "unknown"
		_ = store.GetInstance().UpsertLocalDeployedMicroservice(item)
		return
	}

	switch desired {
	case "stopped":
		pm.reconcileLocalDesiredStopped(item, container, now)
	case "deleted":
		pm.reconcileLocalDesiredDeleted(item, container, now)
	default:
		pm.reconcileLocalDesiredRunning(item, container, now)
	}
}

func (pm *ProcessManager) reconcileLocalDesiredDeleted(item *models.LocalDeployedMicroservice, container *engine.Container, now int64) {
	item.RuntimeState = "deleted"
	item.State = item.RuntimeState
	item.LastTransitionAt = now
	item.ObservedGeneration = item.Generation
	if item.DeletedAt == nil {
		ts := now
		item.DeletedAt = &ts
	}
	if container != nil {
		if err := pm.RemoveContainerByContainerID(container.ID); err != nil {
			item.LastError = err.Error()
			item.RuntimeState = "deleting"
			item.State = item.RuntimeState
			_ = store.GetInstance().UpsertLocalDeployedMicroservice(item)
			return
		}
	}
	item.ContainerID = ""
	item.LastError = ""
	item.FailureCount = 0
	_ = store.GetInstance().UpsertLocalDeployedMicroservice(item)
}

func (pm *ProcessManager) reconcileLocalDesiredStopped(item *models.LocalDeployedMicroservice, container *engine.Container, now int64) {
	item.ObservedGeneration = item.Generation
	item.LastTransitionAt = now
	if container != nil {
		if err := pm.StopMicroservice(item.LocalUUID); err != nil {
			item.LastError = err.Error()
			item.RuntimeState = "stopping"
			item.State = item.RuntimeState
			_ = store.GetInstance().UpsertLocalDeployedMicroservice(item)
			return
		}
	}
	item.RuntimeState = "stopped"
	item.State = item.RuntimeState
	item.LastError = ""
	item.FailureCount = 0
	_ = store.GetInstance().UpsertLocalDeployedMicroservice(item)
}

func (pm *ProcessManager) reconcileLocalDesiredRunning(item *models.LocalDeployedMicroservice, container *engine.Container, now int64) {
	if container == nil {
		pm.launchLocalDeployment(item, now)
		return
	}

	status, err := pm.engine.GetContainerStatus(container.ID, item.LocalUUID)
	if err != nil {
		item.RuntimeState = "unknown"
		item.State = item.RuntimeState
		item.LastError = err.Error()
		item.LastTransitionAt = now
		_ = store.GetInstance().UpsertLocalDeployedMicroservice(item)
		return
	}

	runtime := strings.ToLower(string(status.Status))
	item.RuntimeState = runtime
	item.State = runtime
	item.ContainerID = container.ID
	item.LastTransitionAt = now

	switch runtime {
	case "running":
		item.ObservedGeneration = item.Generation
		item.LastError = ""
		item.FailureCount = 0
	case "created":
		if err := pm.StartMicroservice(item.LocalUUID); err != nil {
			pm.bumpLocalFailure(item, err, "created")
		} else {
			item.RuntimeState = "running"
			item.State = item.RuntimeState
			item.ObservedGeneration = item.Generation
			item.LastError = ""
			item.FailureCount = 0
		}
	case "failed", "exiting", "unknown":
		pm.bumpLocalFailure(item, fmt.Errorf("runtime state=%s", runtime), runtime)
	default:
		if status.ErrorMessage != nil {
			item.LastError = strings.TrimSpace(*status.ErrorMessage)
		}
	}

	_ = store.GetInstance().UpsertLocalDeployedMicroservice(item)
}

func (pm *ProcessManager) bumpLocalFailure(item *models.LocalDeployedMicroservice, cause error, runtime string) {
	item.FailureCount++
	item.RestartCount++
	if cause != nil {
		item.LastError = cause.Error()
	}
	if item.FailureCount >= localReconcileMaxFailures {
		item.RuntimeState = "stuck_in_restart"
		item.State = item.RuntimeState
		return
	}
	item.RuntimeState = runtime
	item.State = item.RuntimeState
}

func (pm *ProcessManager) launchLocalDeployment(item *models.LocalDeployedMicroservice, now int64) {
	doc := &models.LocalDeployManifest{}
	dec := yaml.NewDecoder(strings.NewReader(item.ManifestYAML))
	dec.KnownFields(true)
	if err := dec.Decode(doc); err != nil {
		item.RuntimeState = "failed"
		item.State = item.RuntimeState
		pm.bumpLocalFailure(item, fmt.Errorf("invalid persisted manifest: %w", err), item.RuntimeState)
		item.LastTransitionAt = now
		_ = store.GetInstance().UpsertLocalDeployedMicroservice(item)
		return
	}
	if err := doc.Validate(); err != nil {
		item.RuntimeState = "failed"
		item.State = item.RuntimeState
		pm.bumpLocalFailure(item, fmt.Errorf("invalid persisted manifest: %w", err), item.RuntimeState)
		item.LastTransitionAt = now
		_ = store.GetInstance().UpsertLocalDeployedMicroservice(item)
		return
	}

	arch := strings.ToLower(strings.TrimSpace(config.GetInstance().Arch))
	image := doc.ResolveImageForArch(arch)
	localMS := models.BuildMicroserviceFromLocalManifest(doc, item.LocalUUID, image)
	registry := models.NewRegistry(2, "from_cache", true, "", "", "")
	if doc.Spec.Images.Registry != nil {
		if reg, err := store.GetInstance().GetLocalRegistry(*doc.Spec.Images.Registry); err == nil && reg != nil {
			registry = reg
			localMS.RegistryID = reg.ID
		}
	}

	item.LastStartAttemptAt = now
	hostIP := network.GetInstance().GetCurrentIPAddress()
	containerID, err := pm.LaunchLocalMicroservice(localMS, registry, hostIP)
	if err != nil {
		item.RuntimeState = "failed"
		item.State = item.RuntimeState
		pm.bumpLocalFailure(item, err, item.RuntimeState)
		item.LastTransitionAt = now
		_ = store.GetInstance().UpsertLocalDeployedMicroservice(item)
		return
	}
	item.ContainerID = containerID
	item.ImageName = image
	item.RuntimeState = "running"
	item.State = item.RuntimeState
	item.ObservedGeneration = item.Generation
	item.LastError = ""
	item.FailureCount = 0
	item.LastTransitionAt = now
	_ = store.GetInstance().UpsertLocalDeployedMicroservice(item)
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
				existing := statusreporter.GetInstance().GetProcessManagerStatus().GetMicroserviceStatus(ms.MicroserviceUUID)
				if forceRecreate, reason, exitCode := shouldForceRecreateFromStatus(existing); forceRecreate {
					pm.logger.Warnf(
						"reconcile bypassing stuck gate for terminal runtime state msUUID=%s containerID=%s reason=%s exitCode=%d decision=recreate",
						ms.MicroserviceUUID,
						existing.ContainerID,
						reason,
						exitCode,
					)
					ms.IsStuckInRestart = false
				} else {
					pm.logger.Debugf("Skipping stuck microservice %s (rebuild not requested)", ms.MicroserviceUUID)
					continue
				}
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
		checker := GetRestartStuckChecker()
		if forceRecreate, reason, exitCode := shouldForceRecreateFromStatus(status); forceRecreate {
			sandboxID, _ := pm.engine.GetContainerSandboxID(container.ID)
			pm.logger.Warnf(
				"reconcile detected non-restartable terminal state msUUID=%s containerID=%s sandboxID=%s reason=%s exitCode=%d criMessage=%q decision=recreate",
				ms.MicroserviceUUID,
				container.ID,
				sandboxID,
				reason,
				exitCode,
				safeErrorMessage(status),
			)
			ms.IsStuckInRestart = false
			statusreporter.GetInstance().UpdateProcessManagerStatus(func(s *models.ProcessManagerStatus) {
				s.SetMicroservicesState(ms.MicroserviceUUID, models.MicroserviceStateUpdating)
			})
			pm.addTask(NewContainerTask(TaskActionUpdate, ms.MicroserviceUUID))
			continue
		}
		if status.Status == models.MicroserviceStateCreated {
			sandboxID, _ := pm.engine.GetContainerSandboxID(container.ID)
			pm.logger.Infof(
				"reconcile observed created container msUUID=%s containerID=%s sandboxID=%s criMessage=%q decision=start",
				ms.MicroserviceUUID,
				container.ID,
				sandboxID,
				safeErrorMessage(status),
			)
			if startErr := pm.containerManager.StartContainerByMicroserviceUUID(ms.MicroserviceUUID); startErr != nil {
				if nr, ok := engine.IsNonRestartableContainerError(startErr); ok {
					pm.logger.Warnf(
						"reconcile created start failed with non-restartable terminal state msUUID=%s containerID=%s sandboxID=%s reason=%s exitCode=%d criMessage=%q decision=recreate",
						ms.MicroserviceUUID,
						container.ID,
						sandboxID,
						nr.Reason,
						nr.ExitCode,
						nr.Message,
					)
					ms.IsStuckInRestart = false
					statusreporter.GetInstance().UpdateProcessManagerStatus(func(s *models.ProcessManagerStatus) {
						s.SetMicroservicesState(ms.MicroserviceUUID, models.MicroserviceStateUpdating)
					})
					pm.addTask(NewContainerTask(TaskActionUpdate, ms.MicroserviceUUID))
					continue
				}
				if checker.IsStuckInContainerCreation(ms.MicroserviceUUID) {
					pm.logger.Warnf(
						"reconcile created start failed repeatedly msUUID=%s containerID=%s sandboxID=%s err=%v decision=mark_stuck_in_restart",
						ms.MicroserviceUUID,
						container.ID,
						sandboxID,
						startErr,
					)
					status.Status = models.MicroserviceStateStuckInRestart
					ms.IsStuckInRestart = true
					stuckMsg := stuckInRestartErrorMessage(ms.MicroserviceUUID, fmt.Sprintf("Container repeatedly failing to start: %v", startErr))
					status.ErrorMessage = &stuckMsg
					statusreporter.GetInstance().UpdateProcessManagerStatus(func(pmStatus *models.ProcessManagerStatus) {
						pmStatus.SetMicroservicesStatus(ms.MicroserviceUUID, status)
					})
					continue
				}
				pm.logger.Warnf(
					"reconcile created start failed msUUID=%s containerID=%s sandboxID=%s err=%v decision=retry_start",
					ms.MicroserviceUUID,
					container.ID,
					sandboxID,
					startErr,
				)
			} else {
				pm.logger.Infof(
					"reconcile created start succeeded msUUID=%s containerID=%s sandboxID=%s decision=started",
					ms.MicroserviceUUID,
					container.ID,
					sandboxID,
				)
			}
			continue
		}

		// Detect containers stuck in exit/creation loops and mark them accordingly.
		// Prefer existing error message from engine (e.g. Docker) when available; use static fallback otherwise (matching Java DockerUtil.getMicroserviceStatus).
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
		models.MicroserviceStateStuckInRestart:
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

func shouldForceRecreateFromStatus(status *models.MicroserviceStatus) (bool, string, int32) {
	reason, exitCode := parseCRIReasonAndExitCode(safeErrorMessage(status))
	if engine.IsNonRestartableCRIReason(reason) {
		return true, reason, exitCode
	}
	return false, "", 0
}

func safeErrorMessage(status *models.MicroserviceStatus) string {
	if status == nil || status.ErrorMessage == nil {
		return ""
	}
	return strings.TrimSpace(*status.ErrorMessage)
}

func parseCRIReasonAndExitCode(message string) (string, int32) {
	trimmed := strings.TrimSpace(message)
	if trimmed == "" {
		return "", 0
	}
	reason := ""
	if idx := strings.Index(trimmed, "CRI reason="); idx >= 0 {
		start := idx + len("CRI reason=")
		end := len(trimmed)
		if space := strings.Index(trimmed[start:], " "); space >= 0 {
			end = start + space
		}
		reason = strings.TrimSpace(trimmed[start:end])
	}
	exitCode := int32(0)
	if idx := strings.Index(trimmed, "exitCode="); idx >= 0 {
		start := idx + len("exitCode=")
		end := len(trimmed)
		if space := strings.Index(trimmed[start:], " "); space >= 0 {
			end = start + space
		}
		if parsed, err := strconv.ParseInt(strings.TrimSpace(trimmed[start:end]), 10, 32); err == nil {
			exitCode = int32(parsed)
		}
	}
	return reason, exitCode
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
		msUUID := workloadmeta.MicroserviceUIDFromLabels(container.Labels)
		if msUUID == "" {
			continue
		}

		isCurrent := currentUUIDs[msUUID]
		isLatest := latestUUIDs[msUUID]

		isSystem := workloadmeta.IsSystemWorkload(container.Labels)

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

// updateRunningMicroservicesCount updates the count of running managed microservices.
func (pm *ProcessManager) updateRunningMicroservicesCount() {
	pm.logger.Debug("Update running microservice count")

	containers, err := pm.engine.GetRunningContainers()
	if err != nil {
		pm.logger.Errorf("Error getting running containers: %v", err)
		return
	}

	count := 0
	count = countManagedRunningContainers(containers)

	statusreporter.GetInstance().UpdateProcessManagerStatus(func(status *models.ProcessManagerStatus) {
		status.SetRunningMicroservicesCount(count)
	})
	pm.logger.Debugf("Updated running microservices count: %d", count)
}

func countManagedRunningContainers(containers []engine.Container) int {
	count := 0
	for _, c := range containers {
		if workloadmeta.IsManagedByIofog(c.Labels) {
			count++
		}
	}
	return count
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

// GetContainerForMicroservice resolves runtime container by microservice UUID.
func (pm *ProcessManager) GetContainerForMicroservice(microserviceUUID string) (*engine.Container, error) {
	if pm.containerManager == nil {
		return nil, fmt.Errorf("process manager is not initialized")
	}
	return pm.containerManager.GetContainerForMicroservice(microserviceUUID)
}

// GetContainerByIDPrefix resolves a container by ID prefix.
// Returns the matched container and a list of matching IDs for ambiguity reporting.
func (pm *ProcessManager) GetContainerByIDPrefix(prefix string) (*engine.Container, []string, error) {
	if pm.engine == nil {
		return nil, nil, fmt.Errorf("process manager engine is not initialized")
	}
	trimmed := strings.TrimSpace(prefix)
	if trimmed == "" {
		return nil, nil, fmt.Errorf("container id prefix is required")
	}
	all, err := pm.engine.GetAllContainers()
	if err != nil {
		return nil, nil, err
	}
	matches := make([]engine.Container, 0)
	ids := make([]string, 0)
	for _, cont := range all {
		if strings.HasPrefix(cont.ID, trimmed) {
			matches = append(matches, cont)
			ids = append(ids, cont.ID)
		}
	}
	switch len(matches) {
	case 0:
		return nil, nil, nil
	case 1:
		c := matches[0]
		return &c, ids, nil
	default:
		sort.Strings(ids)
		return nil, ids, nil
	}
}

// GetContainerByID resolves one container by concrete container id.
func (pm *ProcessManager) GetContainerByID(containerID string) (*engine.Container, error) {
	if pm.engine == nil {
		return nil, fmt.Errorf("process manager engine is not initialized")
	}
	return pm.engine.GetContainerByID(containerID)
}

// StartMicroservice starts a runtime microservice container.
func (pm *ProcessManager) StartMicroservice(microserviceUUID string) error {
	if pm.containerManager == nil {
		return fmt.Errorf("process manager is not initialized")
	}
	return pm.containerManager.StartContainerByMicroserviceUUID(microserviceUUID)
}

// StopMicroservice stops a runtime microservice container.
func (pm *ProcessManager) StopMicroservice(microserviceUUID string) error {
	if pm.containerManager == nil {
		return fmt.Errorf("process manager is not initialized")
	}
	return pm.containerManager.StopContainerByMicroserviceUUID(microserviceUUID)
}

// KillMicroservice sends a forceful kill signal to a runtime microservice container.
func (pm *ProcessManager) KillMicroservice(microserviceUUID string) error {
	if pm.containerManager == nil {
		return fmt.Errorf("process manager is not initialized")
	}
	return pm.containerManager.KillContainerByMicroserviceUUID(microserviceUUID)
}

// RestartMicroservice performs stop then start for a runtime microservice container.
func (pm *ProcessManager) RestartMicroservice(microserviceUUID string) error {
	if err := pm.StopMicroservice(microserviceUUID); err != nil {
		return err
	}
	return pm.StartMicroservice(microserviceUUID)
}

// RemoveMicroservice removes a runtime microservice container.
func (pm *ProcessManager) RemoveMicroservice(microserviceUUID string) error {
	if pm.containerManager == nil {
		return fmt.Errorf("process manager is not initialized")
	}
	return pm.containerManager.RemoveContainerByMicroserviceUUID(microserviceUUID, false, false)
}

// RemoveContainerByContainerID removes a runtime container by concrete container id.
func (pm *ProcessManager) RemoveContainerByContainerID(containerID string) error {
	if pm.containerManager == nil {
		return fmt.Errorf("process manager is not initialized")
	}
	return pm.containerManager.RemoveContainerByID(containerID, false, false)
}

// GetMicroserviceUUIDForContainer derives a microservice selector from a container.
func (pm *ProcessManager) GetMicroserviceUUIDForContainer(container engine.Container) string {
	if pm.engine == nil {
		return ""
	}
	return pm.engine.GetContainerMicroserviceUUID(container)
}

// ListImages returns local runtime images from the active engine.
func (pm *ProcessManager) ListImages() ([]engine.ImageInfo, error) {
	if pm.engine == nil {
		return nil, fmt.Errorf("process manager engine is not initialized")
	}
	return pm.engine.ListImages(context.Background())
}

// PullImage pulls an image using optional registry credentials and platform selector.
func (pm *ProcessManager) PullImage(imageRef string, registry *models.Registry, platform string) error {
	if pm.engine == nil {
		return fmt.Errorf("process manager engine is not initialized")
	}
	return pm.engine.PullImage(imageRef, registry, &engine.PullImageOptions{Platform: platform})
}

// PullImageWithProgress pulls an image and reports progress percent when available.
func (pm *ProcessManager) PullImageWithProgress(imageRef string, registry *models.Registry, platform string, onProgress func(float32)) error {
	if pm.engine == nil {
		return fmt.Errorf("process manager engine is not initialized")
	}
	return pm.engine.PullImage(imageRef, registry, &engine.PullImageOptions{
		Platform:         platform,
		ProgressCallback: onProgress,
	})
}

// LoadImageFromPath imports an image archive from daemon-local path.
func (pm *ProcessManager) LoadImageFromPath(path string) ([]engine.LoadedImage, error) {
	if pm.engine == nil {
		return nil, fmt.Errorf("process manager engine is not initialized")
	}
	return pm.engine.LoadImageFromPath(context.Background(), path)
}

// RemoveImage removes an image by ID or name reference.
func (pm *ProcessManager) RemoveImage(selector string) error {
	if pm.engine == nil {
		return fmt.Errorf("process manager engine is not initialized")
	}
	return pm.engine.DeleteImage(context.Background(), selector)
}

// PruneDanglingImages prunes dangling images.
func (pm *ProcessManager) PruneDanglingImages() (*engine.ImagePruneReport, error) {
	if pm.engine == nil {
		return nil, fmt.Errorf("process manager engine is not initialized")
	}
	return pm.engine.PruneDangling(context.Background())
}

// PruneContainers prunes stopped/orphaned containers.
func (pm *ProcessManager) PruneContainers() (*engine.ContainerPruneReport, error) {
	if pm.engine == nil {
		return nil, fmt.Errorf("process manager engine is not initialized")
	}
	return pm.engine.PruneContainers(context.Background())
}

// PruneVolumes prunes unused/orphaned volume artifacts.
func (pm *ProcessManager) PruneVolumes() (*engine.VolumePruneReport, error) {
	if pm.engine == nil {
		return nil, fmt.Errorf("process manager engine is not initialized")
	}
	return pm.engine.PruneVolumes(context.Background())
}

// InspectContainerRaw returns full engine-native inspect payload for a container.
func (pm *ProcessManager) InspectContainerRaw(containerID string) (map[string]interface{}, error) {
	if pm.engine == nil {
		return nil, fmt.Errorf("process manager engine is not initialized")
	}
	return pm.engine.InspectContainerRaw(containerID)
}

// LaunchLocalMicroservice creates and starts a locally deployed microservice.
func (pm *ProcessManager) LaunchLocalMicroservice(ms *models.Microservice, registry *models.Registry, hostIP string) (string, error) {
	return pm.LaunchLocalMicroserviceWithProgress(ms, registry, hostIP, nil)
}

// LaunchLocalMicroserviceWithProgress creates and starts a locally deployed microservice
// while reporting stage transitions.
func (pm *ProcessManager) LaunchLocalMicroserviceWithProgress(ms *models.Microservice, registry *models.Registry, hostIP string, progress LocalDeployProgressCallback) (string, error) {
	if pm.engine == nil {
		return "", fmt.Errorf("process manager engine is not initialized")
	}
	if ms == nil {
		return "", fmt.Errorf("microservice is nil")
	}
	if strings.TrimSpace(hostIP) == "" {
		hostIP = network.GetInstance().GetCurrentIPAddress()
	}
	if registry != nil {
		emitLocalDeployProgress(progress, "pulling", "resolving and preparing image")
		fromCache := strings.EqualFold(strings.TrimSpace(registry.URL), "from_cache")
		pullRef, lookupRefs := imageref.Resolve(ms.ImageName, registry.URL, fromCache)
		pullSucceeded := false
		opts := &engine.PullImageOptions{Platform: msPlatform(ms)}
		if !fromCache {
			if err := pm.engine.PullImage(pullRef, registry, opts); err != nil {
				pm.logger.Warnf("local pull failed for %s, continuing with cache: %v", pullRef, err)
			} else {
				pullSucceeded = true
			}
		}
		matchedRef := ""
		for _, ref := range lookupRefs {
			exists, err := pm.engine.FindLocalImage(ref)
			if err != nil {
				return "", err
			}
			if exists {
				matchedRef = ref
				break
			}
		}
		if matchedRef == "" {
			return "", fmt.Errorf("image not found in local cache for refs: %v", lookupRefs)
		}
		if pullSucceeded {
			ms.ImageName = pullRef
		} else {
			ms.ImageName = matchedRef
		}
	}
	emitLocalDeployProgress(progress, "creating", "creating container")
	containerID, err := pm.engine.CreateContainer(ms, hostIP)
	if err != nil {
		return "", err
	}
	if sandboxID, _ := pm.engine.GetContainerSandboxID(containerID); sandboxID != "" {
		_ = store.GetInstance().SaveLocalContainerState(ms.MicroserviceUUID, containerID, sandboxID)
	}
	emitLocalDeployProgress(progress, "starting", "starting container")
	if err := pm.engine.StartContainer(containerID); err != nil {
		_ = pm.engine.RemoveContainer(containerID, false)
		_ = store.GetInstance().DeleteLocalContainerState(ms.MicroserviceUUID)
		return "", fmt.Errorf("failed to start local microservice runtime: %w", err)
	}
	return containerID, nil
}

// TailMicroserviceLogs returns bounded logs for one runtime microservice.
func (pm *ProcessManager) TailMicroserviceLogs(microserviceUUID string, cfg *engine.TailConfig) ([]map[string]interface{}, error) {
	container, err := pm.GetContainerForMicroservice(microserviceUUID)
	if err != nil {
		return nil, err
	}
	if container == nil {
		return nil, fmt.Errorf("container not found for microservice: %s", microserviceUUID)
	}
	handler := &collectLogTailHandler{
		entries: make([]collectedLogLine, 0),
		done:    make(chan struct{}),
	}
	sessionID := fmt.Sprintf("localapi-log-%d", time.Now().UnixNano())
	if err := pm.engine.TailContainerLogs(container.ID, sessionID, microserviceUUID, handler, cfg); err != nil {
		return nil, err
	}
	// Some engines stream bounded logs asynchronously; wait for completion so
	// non-follow requests return collected lines instead of racing empty output.
	select {
	case <-handler.done:
	case <-time.After(5 * time.Second):
	}
	if handler.err != nil {
		return nil, handler.err
	}
	result := make([]map[string]interface{}, 0, len(handler.entries))
	for _, item := range handler.entries {
		result = append(result, map[string]interface{}{
			"ts":     item.ts,
			"stream": item.stream,
			"line":   item.line,
		})
	}
	return result, nil
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

type collectedLogLine struct {
	ts     string
	stream string
	line   string
}

type collectLogTailHandler struct {
	mu      sync.Mutex
	entries []collectedLogLine
	err     error
	done    chan struct{}
	once    sync.Once
}

func (h *collectLogTailHandler) OnLogLine(_, _ string, line []byte, st engine.StreamType) {
	stream := "stdout"
	if st == engine.Stderr {
		stream = "stderr"
	}
	h.mu.Lock()
	h.entries = append(h.entries, collectedLogLine{
		ts:     time.Now().UTC().Format(time.RFC3339Nano),
		stream: stream,
		line:   string(line),
	})
	h.mu.Unlock()
}

func (h *collectLogTailHandler) OnComplete(_ string) {
	h.once.Do(func() {
		close(h.done)
	})
}

func (h *collectLogTailHandler) OnError(_ string, err error) {
	h.mu.Lock()
	h.err = err
	h.mu.Unlock()
	h.once.Do(func() {
		close(h.done)
	})
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

// GetExecSessionExitCode returns the exit status for completed exec sessions.
func (pm *ProcessManager) GetExecSessionExitCode(execID string) (int, error) {
	return pm.engine.GetExecSessionExitCode(execID)
}

// ResizeExecSession resizes a running tty exec session.
func (pm *ProcessManager) ResizeExecSession(execID string, cols, rows uint32) error {
	return pm.engine.ResizeExecSession(execID, cols, rows)
}

// StopExecSession kills and deregisters the exec process in the engine.
// Called when the controller closes the WebSocket so the exec ID can be reused.
func (pm *ProcessManager) StopExecSession(_ string, execID string) error {
	return pm.engine.StopExecSession(execID)
}

func msPlatform(ms *models.Microservice) string {
	if ms == nil || ms.Platform == nil {
		return ""
	}
	return strings.TrimSpace(*ms.Platform)
}
