package processmanager

import (
	"fmt"
	"time"

	"github.com/eclipse-iofog/agent-go/internal/config"
	"github.com/eclipse-iofog/agent-go/internal/models"
	"github.com/eclipse-iofog/agent-go/internal/network"
	"github.com/eclipse-iofog/agent-go/internal/statusreporter"
	"github.com/eclipse-iofog/agent-go/internal/utils/logging"
	"github.com/eclipse-iofog/agent-go/internal/volumemount"
	"github.com/eclipse-iofog/agent-go/pkg/docker"
)

const (
	ContainerManagerModuleName = "Container Manager"
)

// ContainerManager manages Docker container operations
type ContainerManager struct {
	docker              *docker.Client
	microserviceManager MicroserviceManagerInterface
	logger              *logging.ModuleLogger
}

// NewContainerManager creates a new ContainerManager
func NewContainerManager(dockerClient *docker.Client, microserviceManager MicroserviceManagerInterface) *ContainerManager {
	return &ContainerManager{
		docker:              dockerClient,
		microserviceManager: microserviceManager,
		logger:              logging.NewModuleLogger(ContainerManagerModuleName),
	}
}

// AddContainer adds a container for a microservice
func (cm *ContainerManager) AddContainer(ms *models.Microservice) error {
	cm.logger.Infof("Add container for microservice: %s", ms.ImageName)

	container, err := cm.docker.GetContainer(ms.MicroserviceUUID)
	if err != nil {
		return err
	}

	if container == nil {
		return cm.createContainer(ms)
	}

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

	platform := "linux/amd64"
	if ms.Platform != nil {
		platform = *ms.Platform
	}

	if registry.URL != "from_cache" {
		// Pull image with progress callback
		progressCallback := func(percentage float32) {
		// Update status reporter with percentage
		statusreporter.GetInstance().UpdateProcessManagerStatus(func(status *models.ProcessManagerStatus) {
			status.SetMicroservicesStatePercentage(ms.MicroserviceUUID, percentage)
		})
		}

		if err := cm.docker.PullImage(ms.ImageName, ms.MicroserviceUUID, platform, registry, progressCallback); err != nil {
			cm.logger.Warnf("Unable to pull \"%s\" from registry. Trying local cache: %v", ms.ImageName, err)
			// Continue with local cache if pull fails
		} else {
			cm.logger.Infof("Successfully pulled image \"%s\" while old container was running", ms.ImageName)
		// Set percentage to 100% via status reporter
		statusreporter.GetInstance().UpdateProcessManagerStatus(func(status *models.ProcessManagerStatus) {
			status.SetMicroservicesStatePercentage(ms.MicroserviceUUID, 100.0)
		})
		}
	}

	// Verify image exists (either pulled or in cache)
	exists, err := cm.docker.FindLocalImage(ms.ImageName)
	if err != nil {
		return err
	}
	if !exists {
		ms.SetIsUpdating(false)
		return fmt.Errorf("image not found: %s. Pull failed and image not in local cache", ms.ImageName)
	}

	// Step 2: Now stop and remove old container (releases ports)
	// Downtime starts here, but it's brief compared to pull time
	if err := cm.RemoveContainerByMicroserviceUuid(ms.MicroserviceUUID, withCleanup); err != nil {
		cm.logger.Warnf("Error removing old container: %v", err)
		// Continue anyway
	}

	// Step 3: Create and start new container (can use same ports now)
	// Pass false to createContainer to skip pulling since we already pulled
	if err := cm.createContainer(ms); err != nil {
		return err
	}

	cm.logger.Debugf("Finished update container for microservice: %s", ms.ImageName)
	return nil
}

// RemoveContainerByMicroserviceUuid removes a container by microservice UUID
// Matching Java: ContainerManager.removeContainerByMicroserviceUuid()
func (cm *ContainerManager) RemoveContainerByMicroserviceUuid(microserviceUUID string, withCleanup bool) error {
	cm.logger.Debugf("Start remove container by microserviceuuid: %s", microserviceUUID)

	container, err := cm.docker.GetContainer(microserviceUUID)
	if err != nil {
		return err
	}

	if container != nil {
		// Stop container first
		if err := cm.docker.StopContainer(container.ID); err != nil {
			cm.logger.Warnf("Error stopping container: %v", err)
		}

		// Remove container
		if err := cm.docker.RemoveContainer(container.ID, withCleanup); err != nil {
			cm.logger.Errorf("Error removing container: %v", err)
			return err
		}
	}

	// Clean up per-microservice volume mounts (matching Java: VolumeMountManager.getInstance().cleanupMicroserviceVolumes())
	volumemount.GetInstance().CleanupMicroserviceVolumes(microserviceUUID)

	cm.logger.Infof("Finished remove container by microserviceuuid: %s", microserviceUUID)
	return nil
}

// StopContainerByMicroserviceUuid stops a container by microservice UUID
func (cm *ContainerManager) StopContainerByMicroserviceUuid(microserviceUUID string) error {
	cm.logger.Debugf("Stop container by microserviceuuid: %s", microserviceUUID)

	container, err := cm.docker.GetContainer(microserviceUUID)
	if err != nil {
		return err
	}

	if container != nil {
		return cm.docker.StopContainer(container.ID)
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

	platform := "linux/amd64"
	if ms.Platform != nil {
		platform = *ms.Platform
	}

	if registry.URL != "from_cache" && pullImage {
		progressCallback := func(percentage float32) {
		// Update status reporter with percentage
		statusreporter.GetInstance().UpdateProcessManagerStatus(func(status *models.ProcessManagerStatus) {
			status.SetMicroservicesStatePercentage(ms.MicroserviceUUID, percentage)
		})
		}

		if err := cm.docker.PullImage(ms.ImageName, ms.MicroserviceUUID, platform, registry, progressCallback); err != nil {
			cm.logger.Warnf("Unable to pull \"%s\" from registry. Trying local cache: %v", ms.ImageName, err)
			// Try again without pulling
			return cm.createContainerWithPull(ms, false)
		}
		// Set percentage to 100% via status reporter
		statusreporter.GetInstance().UpdateProcessManagerStatus(func(status *models.ProcessManagerStatus) {
			status.SetMicroservicesStatePercentage(ms.MicroserviceUUID, 100.0)
		})
	}

	if !pullImage {
		exists, err := cm.docker.FindLocalImage(ms.ImageName)
		if err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("image not found in local cache")
		}
	}

	cm.logger.Infof("Creating container \"%s\"", ms.ImageName)
	// Set status to STARTING via status reporter
	statusreporter.GetInstance().UpdateProcessManagerStatus(func(status *models.ProcessManagerStatus) {
		status.SetMicroservicesState(ms.MicroserviceUUID, models.MicroserviceStateStarting)
	})

	cfg := config.GetInstance()
	// Get hostname from network interface manager
	networkManager := network.GetInstance()
	hostName := networkManager.GetHostName()
	if hostName == "" {
		// Fallback to localhost if hostname is not available
		hostName = "localhost"
		cm.logger.Infof("hostname updated to \"%s\"", hostName)
	}
	_ = cfg

	containerID, err := cm.docker.CreateContainer(ms, hostName)
	if err != nil {
		return err
	}

	ms.ContainerID = containerID

	// Get container IP address
	ip, err := cm.docker.GetContainerIPAddress(containerID)
	if err != nil {
		cm.logger.Warnf("Can't get IP address for container: %v", err)
		ip = "0.0.0.0"
	}
	ms.ContainerIPAddress = &ip

	// Start container
	if err := cm.docker.StartContainer(containerID); err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	// Set status to RUNNING via status reporter
	statusreporter.GetInstance().UpdateProcessManagerStatus(func(status *models.ProcessManagerStatus) {
		status.SetMicroservicesState(ms.MicroserviceUUID, models.MicroserviceStateRunning)
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
