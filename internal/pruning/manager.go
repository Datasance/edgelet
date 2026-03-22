package pruning

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/eclipse-iofog/agent/internal/config"
	"github.com/eclipse-iofog/agent/internal/processmanager"
	"github.com/eclipse-iofog/agent/internal/statusreporter"
	"github.com/eclipse-iofog/agent/internal/utils/logging"
	"github.com/eclipse-iofog/agent/pkg/docker"
)

const (
	moduleName = "Docker Pruning Manager"
)

// Manager manages Docker image pruning
type Manager struct {
	config          *config.Config
	dockerClient    *docker.Client
	processManager  *processmanager.ProcessManager
	isPruning       bool
	mu              sync.Mutex
	ctx             context.Context
	cancel          context.CancelFunc
	thresholdTicker *time.Ticker
	frequencyTicker *time.Ticker
}

var (
	instance *Manager
	once     sync.Once
)

// GetInstance returns the singleton Docker Pruning Manager instance
func GetInstance() *Manager {
	once.Do(func() {
		instance = &Manager{
			config:       config.GetInstance(),
			dockerClient: docker.GetInstance(),
		}
		instance.ctx, instance.cancel = context.WithCancel(context.Background())
	})
	return instance
}

// SetProcessManager sets the ProcessManager instance (called by supervisor)
func (m *Manager) SetProcessManager(pm *processmanager.ProcessManager) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.processManager = pm
}

// Start starts the Docker Pruning Manager
func (m *Manager) Start() error {
	logging.LogInfo(moduleName, "Starting Docker Pruning Manager")

	// Reset context on each start to support supervisor restart cycles
	if m.cancel != nil {
		m.cancel()
	}
	m.ctx, m.cancel = context.WithCancel(context.Background())

	// Start threshold-based pruning (check every 30 minutes)
	m.thresholdTicker = time.NewTicker(30 * time.Minute)
	go m.thresholdPruningWorker()

	// Start frequency-based pruning if configured
	pruningFrequency := m.config.DockerPruningFrequency
	if pruningFrequency > 0 {
		duration := time.Duration(pruningFrequency) * time.Hour
		m.frequencyTicker = time.NewTicker(duration)
		go m.frequencyPruningWorker()
		logging.LogInfo(moduleName, fmt.Sprintf("Docker pruning manager started with frequency: %d hours", pruningFrequency))
	} else {
		logging.LogInfo(moduleName, "Docker pruning manager started without frequency-based pruning (frequency set to 0)")
	}

	return nil
}

// Stop stops the Docker Pruning Manager
func (m *Manager) Stop() error {
	logging.LogInfo(moduleName, "Stopping Docker Pruning Manager")
	if m.cancel != nil {
		m.cancel()
	}
	if m.thresholdTicker != nil {
		m.thresholdTicker.Stop()
	}
	if m.frequencyTicker != nil {
		m.frequencyTicker.Stop()
	}
	return nil
}

// thresholdPruningWorker checks disk threshold and prunes if needed
func (m *Manager) thresholdPruningWorker() {
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-m.thresholdTicker.C:
			m.triggerPruneOnThresholdBreach()
		}
	}
}

// frequencyPruningWorker prunes on frequency interval
func (m *Manager) frequencyPruningWorker() {
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-m.frequencyTicker.C:
			m.triggerPruneOnFrequency()
		}
	}
}

// triggerPruneOnThresholdBreach triggers prune when available disk is below threshold
func (m *Manager) triggerPruneOnThresholdBreach() {
	m.mu.Lock()
	if m.isPruning {
		m.mu.Unlock()
		return
	}
	m.mu.Unlock()

	rcmStatus := statusreporter.GetInstance().GetResourceConsumptionManagerStatus()
	if rcmStatus == nil {
		return
	}

	// AvailableDiskThreshold is in MB; AvailableDisk is in bytes
	thresholdBytes := m.config.AvailableDiskThreshold * 1024 * 1024
	if rcmStatus.AvailableDisk < thresholdBytes {
		m.mu.Lock()
		m.isPruning = true
		m.mu.Unlock()
		defer func() {
			m.mu.Lock()
			m.isPruning = false
			m.mu.Unlock()
		}()

		logging.LogInfo(moduleName, "Disk threshold breached, pruning Docker images")
		unwantedImages := m.getUnwantedImagesList()
		if len(unwantedImages) > 0 {
			m.removeImagesByID(unwantedImages)
		}
	}
}

// triggerPruneOnFrequency triggers prune on frequency interval
func (m *Manager) triggerPruneOnFrequency() {
	m.mu.Lock()
	if m.isPruning {
		m.mu.Unlock()
		return
	}
	m.isPruning = true
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		m.isPruning = false
		m.mu.Unlock()
	}()

	logging.LogInfo(moduleName, "Start docker pruning job")

	// TODO: Get unwanted images list and remove them
	// This requires ProcessManager integration
	unwantedImages := m.getUnwantedImagesList()
	if len(unwantedImages) > 0 {
		m.removeImagesByID(unwantedImages)
	}

	logging.LogInfo(moduleName, "Pruning of unwanted images as frequency interval finished")
}

// getUnwantedImagesList gets list of unwanted docker images to be removed
func (m *Manager) getUnwantedImagesList() []string {
	if m.processManager == nil {
		logging.LogWarn(moduleName, "ProcessManager not set, cannot get unwanted images list")
		return []string{}
	}

	// Get all Docker images
	images, err := m.dockerClient.GetImages()
	if err != nil {
		logging.LogError(moduleName, "Failed to get Docker images", err)
		return []string{}
	}
	logging.LogDebug(moduleName, fmt.Sprintf("Total number of images already downloaded: %d", len(images)))

	// Get all running containers
	containers, err := m.dockerClient.GetRunningContainers()
	if err != nil {
		logging.LogError(moduleName, "Failed to get running containers", err)
		return []string{}
	}

	// Get non-ioFog containers (containers not managed by ioFog)
	nonIoFogContainers := make([]docker.Container, 0)
	for _, cont := range containers {
		// Check if container is managed by ioFog (has ioFog label)
		isIoFog := false
		for key := range cont.Labels {
			if key == "iofog-uuid" || key == "iofog.uuid" {
				isIoFog = true
				break
			}
		}
		if !isIoFog {
			nonIoFogContainers = append(nonIoFogContainers, cont)
		}
	}
	logging.LogDebug(moduleName, fmt.Sprintf("Total number of running non-ioFog containers: %d", len(nonIoFogContainers)))

	// Get all used image IDs
	usedImageIDs := make(map[string]bool)

	// Add images used by non-ioFog containers
	for _, cont := range nonIoFogContainers {
		// Extract image ID from image name or use container image directly
		imageID := cont.Image
		// Try to find full image ID from images list
		for _, img := range images {
			for _, tag := range img.RepoTags {
				if tag == cont.Image || strings.HasPrefix(tag, cont.Image+":") {
					imageID = img.ID
					break
				}
			}
		}
		usedImageIDs[imageID] = true
	}

	// Get running ioFog microservices
	if m.processManager != nil {
		microservices := m.processManager.GetLatestMicroservices()
		logging.LogInfo(moduleName, fmt.Sprintf("Total number of running microservices: %d", len(microservices)))

		// Add images used by microservices
		for _, ms := range microservices {
			imageName := ms.ImageName
			// Find image ID for this image name
			for _, img := range images {
				for _, tag := range img.RepoTags {
					if tag == imageName || strings.HasPrefix(tag, imageName+":") {
						usedImageIDs[img.ID] = true
						break
					}
				}
			}
		}
	}

	// Identify prunable images
	imageIDsToBePruned := make([]string, 0)

	for _, img := range images {
		// Check if image is used
		if usedImageIDs[img.ID] {
			continue
		}

		// Add to prunable list
		imageIDsToBePruned = append(imageIDsToBePruned, img.ID)
	}

	logging.LogInfo(moduleName, fmt.Sprintf("Total number of images: %d", len(images)))
	logging.LogInfo(moduleName, fmt.Sprintf("Number of used images: %d", len(usedImageIDs)))
	logging.LogInfo(moduleName, fmt.Sprintf("Total number of unwanted images to be pruned: %d", len(imageIDsToBePruned)))

	return imageIDsToBePruned
}

// removeImagesByID removes images by their IDs
func (m *Manager) removeImagesByID(imageIDs []string) {
	logging.LogInfo(moduleName, fmt.Sprintf("Start removing images by ID, size: %d", len(imageIDs)))
	for _, id := range imageIDs {
		logging.LogInfo(moduleName, fmt.Sprintf("Removing unwanted image id: %s", id))
		if err := m.dockerClient.RemoveImage(id); err != nil {
			logging.LogError(moduleName, fmt.Sprintf("Error removing unwanted docker image id: %s", id), err)
		}
	}
	logging.LogInfo(moduleName, "Finished removing images by ID")
}

// PruneAgent prunes agent through command line (on-demand)
func (m *Manager) PruneAgent() string {
	logging.LogInfo(moduleName, "Initiate prune agent on demand")

	// Get unwanted images and remove them
	unwantedImages := m.getUnwantedImagesList()
	if len(unwantedImages) > 0 {
		m.removeImagesByID(unwantedImages)
		logging.LogInfo(moduleName, fmt.Sprintf("Pruned %d dangling docker images", len(unwantedImages)))
		return fmt.Sprintf("\nSuccess - pruned %d dangling docker images", len(unwantedImages))
	}

	logging.LogInfo(moduleName, "No dangling docker images to prune")
	return "\nSuccess - no dangling docker images to prune"
}

// ChangePruningFreqInterval reschedules frequency-based pruning with new interval
func (m *Manager) ChangePruningFreqInterval() {
	if m.frequencyTicker != nil {
		m.frequencyTicker.Stop()
		m.frequencyTicker = nil
	}

	pruningFrequency := m.config.DockerPruningFrequency
	if pruningFrequency > 0 {
		duration := time.Duration(pruningFrequency) * time.Hour
		m.frequencyTicker = time.NewTicker(duration)
		go m.frequencyPruningWorker()
		logging.LogInfo(moduleName, fmt.Sprintf("Docker pruning frequency updated to: %d hours", pruningFrequency))
	} else {
		logging.LogInfo(moduleName, "Docker pruning frequency set to 0 - frequency-based pruning disabled")
	}
}

// GetName returns the module name
func (m *Manager) GetName() string {
	return moduleName
}

// GetModuleIndex returns the module index (Docker Pruning Manager doesn't have a specific index)
func (m *Manager) GetModuleIndex() int {
	return -1 // Docker Pruning Manager is not tracked in status
}
