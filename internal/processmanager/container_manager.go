package processmanager

import (
	"errors"
	"fmt"
	"time"

	"github.com/eclipse-iofog/agent/internal/config"
	"github.com/eclipse-iofog/agent/internal/models"
	"github.com/eclipse-iofog/agent/internal/network"
	"github.com/eclipse-iofog/agent/internal/statusreporter"
	"github.com/eclipse-iofog/agent/internal/store"
	"github.com/eclipse-iofog/agent/internal/utils/logging"
	"github.com/eclipse-iofog/agent/internal/volumemount"
	"github.com/eclipse-iofog/agent/internal/workloadmeta"
	"github.com/eclipse-iofog/agent/pkg/engine"
	"github.com/eclipse-iofog/agent/pkg/imageref"
)

const (
	ContainerManagerModuleName = "Container Manager"
)

// ContainerManager manages container operations via a ContainerEngine.
type ContainerManager struct {
	engine              engine.ContainerEngine
	microserviceManager MicroserviceManagerInterface
	logger              *logging.ModuleLogger
}

// NewContainerManager creates a new ContainerManager
func NewContainerManager(eng engine.ContainerEngine, microserviceManager MicroserviceManagerInterface) *ContainerManager {
	return &ContainerManager{
		engine:              eng,
		microserviceManager: microserviceManager,
		logger:              logging.NewModuleLogger(ContainerManagerModuleName),
	}
}

// GetContainerForMicroservice returns the container for a microservice, using DB-first
// lookup when available (iofog engine) with label-based fallback.
func (cm *ContainerManager) GetContainerForMicroservice(microserviceUUID string) (*engine.Container, error) {
	if cs, err := store.GetInstance().GetContainerState(microserviceUUID); err == nil && cs != nil && cs.WorkloadID != "" {
		if c, err := cm.engine.GetContainerByID(cs.WorkloadID); err == nil && c != nil {
			return c, nil
		}
		cm.logger.Warnf(
			"runtime drift cleanup: stale container_state entry msUUID=%s containerID=%s decision=cleanup-stale-db",
			microserviceUUID,
			cs.WorkloadID,
		)
		_ = store.GetInstance().DeleteContainerState(microserviceUUID)
	}
	if cs, err := store.GetInstance().GetLocalContainerState(microserviceUUID); err == nil && cs != nil && cs.WorkloadID != "" {
		if c, err := cm.engine.GetContainerByID(cs.WorkloadID); err == nil && c != nil {
			return c, nil
		}
		cm.logger.Warnf(
			"runtime drift cleanup: stale local_container_state entry msUUID=%s containerID=%s decision=cleanup-stale-db",
			microserviceUUID,
			cs.WorkloadID,
		)
		_ = store.GetInstance().DeleteLocalContainerState(microserviceUUID)
	}
	c, err := cm.engine.GetContainer(microserviceUUID)
	if err != nil {
		return nil, err
	}
	if c != nil {
		if sandboxID, _ := cm.engine.GetContainerSandboxID(c.ID); sandboxID != "" {
			_ = store.GetInstance().SaveContainerState(microserviceUUID, c.ID, sandboxID)
		}
	}
	return c, nil
}

// AddContainer creates and starts a container for a microservice.
// It holds IsUpdating=true for the duration so the reconciliation loop treats this
// microservice as "in-flight" and does not enqueue a second ADD task.
func (cm *ContainerManager) AddContainer(ms *models.Microservice) error {
	cm.logger.Infof("Add container for microservice: %s", ms.ImageName)

	ms.SetIsUpdating(true)
	defer ms.SetIsUpdating(false)

	container, err := cm.GetContainerForMicroservice(ms.MicroserviceUUID)
	if err != nil {
		return err
	}

	if container == nil {
		return cm.createContainer(ms)
	}

	// Container already exists (created by a concurrent task) — nothing to do.
	return nil
}

// UpdateContainer updates a container for a microservice
func (cm *ContainerManager) UpdateContainer(ms *models.Microservice, withCleanup bool) error {
	cm.logger.Infof("Start update container for microservice: %s", ms.ImageName)

	ms.SetIsUpdating(true)
	defer ms.SetIsUpdating(false)

	// Step 1: Pull new image while old container is still running
	// This keeps the service available during the slow image pull operation
	registry := cm.microserviceManager.GetRegistry(ms.RegistryID)
	if registry == nil {
		return fmt.Errorf("registry is not valid \"%d\"", ms.RegistryID)
	}
	fromCache := registry.URL == "from_cache"
	pullRef, lookupRefs := imageref.Resolve(ms.ImageName, registry.URL, fromCache)
	pullSucceeded := false

	if registry.URL != "from_cache" {
		opts := &engine.PullImageOptions{
			Platform: cm.platformForPull(ms),
			ProgressCallback: func(pct float32) {
				statusreporter.GetInstance().UpdateProcessManagerStatus(func(s *models.ProcessManagerStatus) {
					s.SetMicroservicesStatePercentage(ms.MicroserviceUUID, pct)
				})
			},
		}
		if err := cm.engine.PullImage(pullRef, registry, opts); err != nil {
			cm.logger.Warnf("Unable to pull \"%s\" from registry. Trying local cache: %v", pullRef, err)
		} else {
			pullSucceeded = true
			cm.logger.Infof("Successfully pulled image \"%s\" while old container was running", pullRef)
			statusreporter.GetInstance().UpdateProcessManagerStatus(func(status *models.ProcessManagerStatus) {
				status.SetMicroservicesStatePercentage(ms.MicroserviceUUID, 100.0)
			})
		}
	}

	// Verify image exists (either pulled or in cache) and pick the runtime reference
	// that will be used for container creation.
	matchedRef, exists, err := cm.findFirstLocalImageRef(lookupRefs)
	if err != nil {
		return err
	}
	if !exists {
		ms.SetIsUpdating(false)
		return fmt.Errorf("image not found in local cache for refs: %v", lookupRefs)
	}
	effectiveRunRef := matchedRef
	if pullSucceeded {
		effectiveRunRef = pullRef
	}

	// Step 2: Now stop and remove old container (releases ports)
	// Downtime starts here, but it's brief compared to pull time
	// removeImage=withCleanup: matches Java ContainerManager behavior (image deleted on clean update)
	if err := cm.RemoveContainerByMicroserviceUUID(ms.MicroserviceUUID, withCleanup, withCleanup); err != nil {
		cm.logger.Warnf("Error removing old container: %v", err)
		// Continue anyway
	} else {
		cm.logger.Infof("recreate phase result msUUID=%s phase=remove result=ok", ms.MicroserviceUUID)
	}

	// Step 3: Create and start new container (can use same ports now)
	// Use the resolved runtime image ref so pull/check/create use the same reference.
	ms.ImageName = effectiveRunRef
	if err := cm.createContainer(ms); err != nil {
		cm.logger.Warnf("recreate phase result msUUID=%s phase=create_or_start result=failed err=%v", ms.MicroserviceUUID, err)
		return err
	}
	cm.logger.Infof("recreate phase result msUUID=%s phase=create_or_start result=ok", ms.MicroserviceUUID)

	cm.logger.Debugf("Finished update container for microservice: %s", ms.ImageName)
	return nil
}

// RemoveContainerByMicroserviceUUID removes a container by microservice UUID.
// withCleanup controls Docker named-volume removal (passed to engine.RemoveContainer).
// removeImage controls whether the container image is also removed after container deletion —
// set true for normal lifecycle deletions (matching Java ContainerManager behavior),
// false for the deprovision path (matching Java ProcessManager private method behavior).
func (cm *ContainerManager) RemoveContainerByMicroserviceUUID(microserviceUUID string, withCleanup bool, removeImage bool) error {
	cm.logger.Debugf("Start remove container by microserviceuuid: %s", microserviceUUID)

	container, err := cm.GetContainerForMicroservice(microserviceUUID)
	if err != nil {
		return err
	}
	// Fallback for watchdog/unknown-container flows where the incoming identifier
	// can be a concrete container ID (or other non-iofog name) that doesn't resolve
	// through microservice UUID lookup.
	if container == nil {
		container, err = cm.engine.GetContainerByID(microserviceUUID)
		if err != nil {
			return err
		}
	}

	if container == nil {
		// Container already gone (e.g. crashed) — still report DELETED so controller receives final state
		statusreporter.GetInstance().UpdateProcessManagerStatus(func(s *models.ProcessManagerStatus) {
			s.SetMicroservicesState(microserviceUUID, models.MicroserviceStateDeleted)
			s.SetMicroservicesStatusErrorMessage(microserviceUUID, "")
		})
		_ = store.GetInstance().DeleteContainerState(microserviceUUID)
		_ = store.GetInstance().DeleteLocalContainerState(microserviceUUID)
	} else {
		imageRef := container.Image

		// Set DELETING status before removal (matches Java ContainerManager.setMicroserviceStatus(DELETING))
		statusreporter.GetInstance().UpdateProcessManagerStatus(func(s *models.ProcessManagerStatus) {
			s.SetMicroservicesState(microserviceUUID, models.MicroserviceStateDeleting)
		})

		if err := cm.engine.StopContainer(container.ID); err != nil {
			cm.logger.Warnf("Error stopping container: %v", err)
		}
		if err := cm.engine.RemoveContainer(container.ID, withCleanup); err != nil {
			cm.logger.Errorf("Error removing container: %v", err)
			return err
		}

		// Set DELETED status after successful removal so controller receives final state
		statusreporter.GetInstance().UpdateProcessManagerStatus(func(s *models.ProcessManagerStatus) {
			s.SetMicroservicesState(microserviceUUID, models.MicroserviceStateDeleted)
			s.SetMicroservicesStatusErrorMessage(microserviceUUID, "")
		})

		// Remove image when explicitly requested (matching Java ContainerManager.removeContainer logic).
		// Errors are logged as warnings only — image may still be in use by another container
		// (equivalent to Java catching ConflictException).
		if removeImage && imageRef != "" {
			if err := cm.engine.RemoveImage(imageRef); err != nil {
				cm.logger.Warnf("Image %s cannot be removed (may be in use): %v", imageRef, err)
			}
		}

		// Clear container state from DB (iofog engine uses this for lookup)
		_ = store.GetInstance().DeleteContainerState(microserviceUUID)
		_ = store.GetInstance().DeleteLocalContainerState(microserviceUUID)
	}

	// Clean up per-microservice volume mounts (matching Java: VolumeMountManager.getInstance().cleanupMicroserviceVolumes())
	volumemount.GetInstance().CleanupMicroserviceVolumes(microserviceUUID)

	cm.logger.Infof("Finished remove container by microserviceuuid: %s", microserviceUUID)
	return nil
}

// RemoveContainerByID removes a container by concrete engine-assigned container ID.
// This is primarily used by watchdog unknown-container cleanup where there is no
// guaranteed iofog microservice UUID mapping.
func (cm *ContainerManager) RemoveContainerByID(containerID string, withCleanup bool, removeImage bool) error {
	cm.logger.Debugf("Start remove container by containerID: %s", containerID)

	container, err := cm.engine.GetContainerByID(containerID)
	if err != nil {
		return err
	}
	if container == nil {
		cm.logger.Infof("Container already removed by containerID: %s", containerID)
		return nil
	}

	imageRef := container.Image
	msUUID := ""
	if workloadmeta.IsManagedByIofog(container.Labels) {
		msUUID = workloadmeta.MicroserviceUIDFromLabels(container.Labels)
	}

	if err := cm.engine.StopContainer(container.ID); err != nil {
		cm.logger.Warnf("Error stopping container %s: %v", container.ID, err)
	}
	if err := cm.engine.RemoveContainer(container.ID, withCleanup); err != nil {
		cm.logger.Errorf("Error removing container %s: %v", container.ID, err)
		return err
	}

	if removeImage && imageRef != "" {
		if err := cm.engine.RemoveImage(imageRef); err != nil {
			cm.logger.Warnf("Image %s cannot be removed (may be in use): %v", imageRef, err)
		}
	}

	// Best-effort cleanup for iofog-managed containers.
	if msUUID != "" {
		_ = store.GetInstance().DeleteContainerState(msUUID)
		_ = store.GetInstance().DeleteLocalContainerState(msUUID)
		_ = store.GetInstance().DeleteLocalDeployedMicroservice(msUUID)
		volumemount.GetInstance().CleanupMicroserviceVolumes(msUUID)
	}

	cm.logger.Infof("Finished remove container by containerID: %s", containerID)
	return nil
}

// StopContainerByMicroserviceUUID stops a container by microservice UUID
func (cm *ContainerManager) StopContainerByMicroserviceUUID(microserviceUUID string) error {
	cm.logger.Debugf("Stop container by microserviceuuid: %s", microserviceUUID)

	container, err := cm.GetContainerForMicroservice(microserviceUUID)
	if err != nil {
		return err
	}

	if container != nil {
		return cm.engine.StopContainer(container.ID)
	}

	return nil
}

// StartContainerByMicroserviceUUID starts a container by microservice UUID.
func (cm *ContainerManager) StartContainerByMicroserviceUUID(microserviceUUID string) error {
	cm.logger.Debugf("Start container by microserviceuuid: %s", microserviceUUID)
	container, err := cm.GetContainerForMicroservice(microserviceUUID)
	if err != nil {
		return err
	}
	if container != nil {
		err := cm.engine.StartContainer(container.ID)
		if err == nil {
			return nil
		}
		var nr *engine.NonRestartableContainerError
		if errors.As(err, &nr) {
			cm.logger.Warnf(
				"start classification: non-restartable terminal state msUUID=%s containerID=%s reason=%s exitCode=%d criMessage=%q decision=recreate",
				microserviceUUID,
				nr.ContainerID,
				nr.Reason,
				nr.ExitCode,
				nr.Message,
			)
		}
		return err
	}
	return nil
}

// KillContainerByMicroserviceUUID forcefully stops a container by microservice UUID.
func (cm *ContainerManager) KillContainerByMicroserviceUUID(microserviceUUID string) error {
	cm.logger.Debugf("Kill container by microserviceuuid: %s", microserviceUUID)
	container, err := cm.GetContainerForMicroservice(microserviceUUID)
	if err != nil {
		return err
	}
	if container != nil {
		return cm.engine.KillContainer(container.ID)
	}
	return nil
}

// createContainer creates a container for a microservice
func (cm *ContainerManager) createContainer(ms *models.Microservice) error {
	return cm.createContainerWithPull(ms, true)
}

// createContainerWithPull creates a container, optionally pulling the image first
func (cm *ContainerManager) createContainerWithPull(ms *models.Microservice, pullImage bool) error {
	// Set status to PULLING via status reporter
	statusreporter.GetInstance().UpdateProcessManagerStatus(func(status *models.ProcessManagerStatus) {
		status.SetMicroservicesState(ms.MicroserviceUUID, models.MicroserviceStatePulling)
	})

	registry := cm.microserviceManager.GetRegistry(ms.RegistryID)
	if registry == nil {
		return fmt.Errorf("registry is not valid \"%d\"", ms.RegistryID)
	}
	fromCache := registry.URL == "from_cache"
	pullRef, lookupRefs := imageref.Resolve(ms.ImageName, registry.URL, fromCache)
	pullSucceeded := false

	if registry.URL != "from_cache" && pullImage {
		opts := &engine.PullImageOptions{
			Platform: cm.platformForPull(ms),
			ProgressCallback: func(pct float32) {
				statusreporter.GetInstance().UpdateProcessManagerStatus(func(s *models.ProcessManagerStatus) {
					s.SetMicroservicesStatePercentage(ms.MicroserviceUUID, pct)
				})
			},
		}
		if err := cm.engine.PullImage(pullRef, registry, opts); err != nil {
			cm.logger.Warnf("Unable to pull \"%s\" from registry. Trying local cache: %v", pullRef, err)
			return cm.createContainerWithPull(ms, false)
		}
		pullSucceeded = true
		statusreporter.GetInstance().UpdateProcessManagerStatus(func(status *models.ProcessManagerStatus) {
			status.SetMicroservicesStatePercentage(ms.MicroserviceUUID, 100.0)
		})
	}

	// Verify image exists in local cache and pick the runtime image reference.
	matchedRef, exists, err := cm.findFirstLocalImageRef(lookupRefs)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("image not found in local cache for refs: %v", lookupRefs)
	}
	effectiveRunRef := matchedRef
	if pullSucceeded {
		effectiveRunRef = pullRef
	}
	ms.ImageName = effectiveRunRef

	cm.logger.Infof("Creating container \"%s\"", ms.ImageName)
	// Set status to STARTING via status reporter
	statusreporter.GetInstance().UpdateProcessManagerStatus(func(status *models.ProcessManagerStatus) {
		status.SetMicroservicesState(ms.MicroserviceUUID, models.MicroserviceStateStarting)
		status.SetMicroservicesStatusErrorMessage(ms.MicroserviceUUID, "")
	})

	cfg := config.GetInstance()
	// Get host IP from network interface manager (matches Java: getCurrentIpAddress).
	// Used for iofog and service.local extra hosts so containers can reach the host/agent.
	networkManager := network.GetInstance()
	hostIP := networkManager.GetCurrentIPAddress()
	if hostIP == "" {
		// Retry like Java ContainerManager.retryHostName
		for tries := 0; tries < 5 && hostIP == ""; tries++ {
			time.Sleep(500 * time.Millisecond)
			hostIP = networkManager.GetCurrentIPAddress()
		}
		if hostIP == "" {
			hostIP = "127.0.0.1"
			cm.logger.Infof("host IP unavailable, using fallback %q", hostIP)
		}
	}
	_ = cfg

	containerID, err := cm.engine.CreateContainer(ms, hostIP)
	if err != nil {
		return err
	}

	ms.ContainerID = containerID

	if sandboxID, _ := cm.engine.GetContainerSandboxID(containerID); sandboxID != "" {
		_ = store.GetInstance().SaveContainerState(ms.MicroserviceUUID, containerID, sandboxID)
	}

	ip, err := cm.engine.GetContainerIPAddress(containerID)
	if err != nil {
		cm.logger.Warnf("Can't get IP address for container: %v", err)
		ip = "0.0.0.0"
	}
	ms.ContainerIPAddress = &ip

	if err := cm.engine.StartContainer(containerID); err != nil {
		var nr *engine.NonRestartableContainerError
		if errors.As(err, &nr) {
			cm.logger.Warnf(
				"recreate phase result msUUID=%s phase=start result=failed reason=%s exitCode=%d criMessage=%q containerID=%s",
				ms.MicroserviceUUID,
				nr.Reason,
				nr.ExitCode,
				nr.Message,
				nr.ContainerID,
			)
		}
		return fmt.Errorf("failed to start container: %w", err)
	}
	cm.logger.Infof("recreate phase result msUUID=%s phase=start result=ok containerID=%s", ms.MicroserviceUUID, containerID)

	// Clear rebuild flag after successful creation (matches Java ContainerManager.setRebuild(false))
	ms.Rebuild = false

	// Set status to RUNNING via status reporter
	statusreporter.GetInstance().UpdateProcessManagerStatus(func(status *models.ProcessManagerStatus) {
		status.SetMicroservicesState(ms.MicroserviceUUID, models.MicroserviceStateRunning)
		status.SetMicroservicesStatusErrorMessage(ms.MicroserviceUUID, "")
		// Set start time
		if msStatus := status.GetMicroserviceStatus(ms.MicroserviceUUID); msStatus != nil {
			msStatus.StartTime = time.Now().UnixMilli()
			msStatus.ContainerID = containerID
			if ms.ContainerIPAddress != nil {
				msStatus.IPAddress = ms.ContainerIPAddress
			}
		}
	})
	_ = cfg

	return nil
}

func (cm *ContainerManager) platformForPull(ms *models.Microservice) string {
	if ms == nil || ms.Platform == nil {
		return ""
	}
	return *ms.Platform
}

func (cm *ContainerManager) findFirstLocalImageRef(refs []string) (string, bool, error) {
	for _, ref := range refs {
		exists, err := cm.engine.FindLocalImage(ref)
		if err != nil {
			return "", false, err
		}
		if exists {
			return ref, true, nil
		}
	}
	return "", false, nil
}
