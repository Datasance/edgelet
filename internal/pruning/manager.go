package pruning

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/eclipse-iofog/agent/internal/config"
	"github.com/eclipse-iofog/agent/internal/statusreporter"
	"github.com/eclipse-iofog/agent/internal/utils/logging"
	"github.com/eclipse-iofog/agent/pkg/docker"
	"github.com/eclipse-iofog/agent/pkg/engine"
)

const (
	moduleName = "Docker Pruning Manager"
)

// Manager manages image pruning for all container engine types.
type Manager struct {
	config          *config.Config
	dockerClient    *docker.Client
	containerEngine engine.ContainerEngine // non-nil when a non-Docker engine is active
	// getMicroserviceImages returns the image names of ALL currently configured
	// microservices (running + stopped/restarting). Set by the supervisor via
	// SetGetMicroservicesCallback. Matches Java MicroserviceManager.getLatestMicroservices().
	getMicroserviceImages func() []string
	isPruning             bool
	mu                    sync.Mutex
	ctx                   context.Context
	cancel                context.CancelFunc
	thresholdTicker       *time.Ticker
	frequencyTicker       *time.Ticker
	// freqCtx/freqCancel govern only the frequency worker goroutine so it can
	// be canceled independently when ChangePruningFreqInterval is called
	// without disrupting the threshold worker (which uses the main ctx).
	freqCtx    context.Context
	freqCancel context.CancelFunc
	// lastAppliedPruningFrequency tracks the last frequency value that was applied.
	// It is used to trigger an immediate frequency prune only when pruning is
	// enabled (0 -> N) or the frequency value actually changes (N1 -> N2).
	lastAppliedPruningFrequency int64
}

var (
	instance *Manager
	once     sync.Once
)

// GetInstance returns the singleton pruning Manager instance.
func GetInstance() *Manager {
	once.Do(func() {
		instance = &Manager{
			config:       config.GetInstance(),
			dockerClient: docker.GetInstance(),
		}
		instance.ctx, instance.cancel = context.WithCancel(context.Background())
		instance.freqCtx, instance.freqCancel = context.WithCancel(context.Background())
	})
	return instance
}

// SetGetMicroservicesCallback sets a callback that returns image names for ALL
// currently configured microservices (running or not). These images are protected
// from scheduled/threshold pruning, matching Java DockerPruningManager behavior.
// Called by the supervisor after ProcessManager is wired up.
func (m *Manager) SetGetMicroservicesCallback(fn func() []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.getMicroserviceImages = fn
}

// SetEngine sets the ContainerEngine to use for pruning.
// When set, pruning delegates to engine.PruneImages() instead of the Docker-specific logic,
// enabling correct pruning for iofog/containerd and other non-Docker engines.
func (m *Manager) SetEngine(e engine.ContainerEngine) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.containerEngine = e
}

// Start starts the Docker Pruning Manager
func (m *Manager) Start() error {
	logging.LogInfo(moduleName, "Starting Docker Pruning Manager")

	// Reset contexts on each start to support supervisor restart cycles.
	if m.cancel != nil {
		m.cancel()
	}
	if m.freqCancel != nil {
		m.freqCancel()
	}
	m.ctx, m.cancel = context.WithCancel(context.Background())
	m.freqCtx, m.freqCancel = context.WithCancel(context.Background())

	// Start threshold-based pruning (check every 30 minutes)
	m.thresholdTicker = time.NewTicker(30 * time.Minute)
	go m.thresholdPruningWorker()

	// Start frequency-based pruning if configured (0 = disabled)
	pruningFrequency := m.config.DockerPruningFrequency
	runImmediate := m.shouldRunImmediateFrequencyPrune(pruningFrequency)
	if pruningFrequency > 0 {
		duration := time.Duration(pruningFrequency) * time.Hour
		m.frequencyTicker = time.NewTicker(duration)
		go m.frequencyPruningWorker()
		if runImmediate {
			go m.triggerPruneOnFrequency()
		}
		logging.LogInfo(moduleName, fmt.Sprintf("Docker pruning manager started with frequency: %d hours", pruningFrequency))
	} else {
		logging.LogInfo(moduleName, "Docker pruning manager started without frequency-based pruning (frequency set to 0)")
	}
	m.setLastAppliedPruningFrequency(pruningFrequency)

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

// frequencyPruningWorker prunes on frequency interval.
// It uses freqCtx (not the main ctx) so it can be canceled independently
// by ChangePruningFreqInterval without stopping the threshold worker.
func (m *Manager) frequencyPruningWorker() {
	for {
		select {
		case <-m.freqCtx.Done():
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

	// AvailableDiskThreshold is a percentage (0-100), matching Java's DockerPruningManager.
	// Prune when the available disk percentage falls below the configured threshold.
	// Guard against division by zero when TotalDiskSpace is not yet populated.
	if rcmStatus.TotalDiskSpace <= 0 {
		return
	}
	availablePercent := rcmStatus.AvailableDisk * 100 / rcmStatus.TotalDiskSpace
	if availablePercent < m.config.AvailableDiskThreshold {
		m.mu.Lock()
		m.isPruning = true
		m.mu.Unlock()
		defer func() {
			m.mu.Lock()
			m.isPruning = false
			m.mu.Unlock()
		}()

		logging.LogInfo(moduleName, "Disk threshold breached, pruning images")
		m.pruneImages()
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

	logging.LogInfo(moduleName, "Start pruning job")
	m.pruneImages()
	logging.LogInfo(moduleName, "Pruning of unwanted images finished")
}

// pruneImages removes unused images using the unified getUnwantedImagesList() logic
// for all engine types. This matches Java DockerPruningManager's scheduled/threshold
// pruning: protect images for ALL configured microservices + non-ioFog containers,
// delete everything else.
func (m *Manager) pruneImages() {
	ctx := context.Background()
	m.mu.Lock()
	eng := m.containerEngine
	m.mu.Unlock()

	unwanted := m.getUnwantedImagesList(ctx, eng)
	if len(unwanted) == 0 {
		return
	}
	for _, nameOrID := range unwanted {
		logging.LogInfo(moduleName, fmt.Sprintf("Removing unwanted image: %s", nameOrID))
		var err error
		if eng != nil {
			err = eng.DeleteImage(ctx, nameOrID)
		} else {
			err = m.dockerClient.RemoveImage(nameOrID)
		}
		if err != nil {
			logging.LogError(moduleName, fmt.Sprintf("Error removing image %s", nameOrID), err)
		}
	}
}

// getUnwantedImagesList returns image IDs/names that should be deleted during
// scheduled or threshold pruning. The logic matches Java DockerPruningManager:
//
//   - Protect images used by running non-ioFog containers (by image name/ID).
//   - Protect images for ALL configured microservices via getMicroserviceImages
//     callback (includes stopped/restarting, not just running ones).
//   - Delete every other image.
//
// Works for all engine types via the ContainerEngine interface. When eng is nil
// it falls back to the Docker client.
func (m *Manager) getUnwantedImagesList(ctx context.Context, eng engine.ContainerEngine) []string {
	var allImages []engine.ImageInfo
	var runningContainers []engine.Container

	if eng != nil {
		imgs, err := eng.ListImages(ctx)
		if err != nil {
			logging.LogError(moduleName, "Failed to list images via engine", err)
			return []string{}
		}
		allImages = imgs

		conts, err := eng.GetRunningContainers()
		if err != nil {
			logging.LogError(moduleName, "Failed to get running containers via engine", err)
			return []string{}
		}
		runningContainers = conts
	} else {
		// Docker fallback: map docker.image.Summary → engine.ImageInfo
		dockerImgs, err := m.dockerClient.GetImages()
		if err != nil {
			logging.LogError(moduleName, "Failed to get Docker images", err)
			return []string{}
		}
		allImages = make([]engine.ImageInfo, 0, len(dockerImgs))
		for _, di := range dockerImgs {
			allImages = append(allImages, engine.ImageInfo{ID: di.ID, RepoTags: di.RepoTags})
		}

		dockerConts, err := m.dockerClient.GetRunningContainers()
		if err != nil {
			logging.LogError(moduleName, "Failed to get Docker running containers", err)
			return []string{}
		}
		runningContainers = make([]engine.Container, 0, len(dockerConts))
		for _, dc := range dockerConts {
			labels := make(map[string]string, len(dc.Labels))
			for k, v := range dc.Labels {
				labels[k] = v
			}
			runningContainers = append(runningContainers, engine.Container{
				ID:     dc.ID,
				Image:  dc.Image,
				Labels: labels,
			})
		}
	}

	logging.LogDebug(moduleName, fmt.Sprintf("Total images: %d", len(allImages)))

	// Build a lookup map from tag → image ID for quick resolution.
	tagToID := make(map[string]string, len(allImages))
	for _, img := range allImages {
		for _, tag := range img.RepoTags {
			tagToID[tag] = img.ID
		}
	}

	usedImageIDs := make(map[string]bool)

	// Protect images used by running NON-ioFog containers.
	nonIoFogCount := 0
	for _, cont := range runningContainers {
		isIoFog := false
		for k := range cont.Labels {
			if k == "iofog-uuid" || k == "iofog.uuid" || k == "iofog-ms" {
				isIoFog = true
				break
			}
		}
		if isIoFog {
			continue
		}
		nonIoFogCount++
		if id, ok := tagToID[cont.Image]; ok {
			usedImageIDs[id] = true
		} else {
			usedImageIDs[cont.Image] = true // fallback: use image name directly
		}
	}
	logging.LogDebug(moduleName, fmt.Sprintf("Running non-ioFog containers: %d", nonIoFogCount))

	// Protect images for ALL configured microservices (running + stopped).
	if m.getMicroserviceImages != nil {
		msImages := m.getMicroserviceImages()
		logging.LogInfo(moduleName, fmt.Sprintf("Configured microservice images to protect: %d", len(msImages)))
		for _, imgName := range msImages {
			if id, ok := tagToID[imgName]; ok {
				usedImageIDs[id] = true
			} else {
				usedImageIDs[imgName] = true
			}
		}
	}

	// Collect images not in the protected set.
	toBePruned := make([]string, 0)
	for _, img := range allImages {
		if usedImageIDs[img.ID] {
			continue
		}
		// Use the first tag as the deletion key, fall back to ID.
		key := img.ID
		if len(img.RepoTags) > 0 {
			key = img.RepoTags[0]
		}
		toBePruned = append(toBePruned, key)
	}

	logging.LogInfo(moduleName, fmt.Sprintf("Images total: %d, used: %d, to prune: %d",
		len(allImages), len(usedImageIDs), len(toBePruned)))
	return toBePruned
}

// PruneAgent prunes dangling images on demand (CLI command / controller API).
// Matches Java DockerPruningManager.pruneAgent() which calls docker system prune
// (dangling-only). This is intentionally lighter than scheduled pruning which
// removes all unused images.
func (m *Manager) PruneAgent() string {
	logging.LogInfo(moduleName, "Initiate prune agent on demand")

	m.mu.Lock()
	eng := m.containerEngine
	m.mu.Unlock()

	ctx := context.Background()
	if eng != nil {
		if _, err := eng.PruneDangling(ctx); err != nil {
			logging.LogError(moduleName, "Error pruning dangling images via container engine", err)
			return "\nFailure - error pruning dangling images"
		}
		logging.LogInfo(moduleName, "Pruned dangling images via container engine")
		return "\nSuccess - pruned dangling images"
	}

	// Docker engine: dangling only (docker system prune)
	_, err := m.dockerClient.DockerPrune()
	if err != nil {
		logging.LogError(moduleName, "Error pruning dangling Docker images", err)
		return "\nFailure - error pruning dangling images"
	}
	logging.LogInfo(moduleName, "Pruned dangling Docker images")
	return "\nSuccess - pruned dangling images"
}

// ChangePruningFreqInterval reschedules frequency-based pruning with new interval.
// Cancels the old frequency goroutine before launching a new one to prevent leaks.
func (m *Manager) ChangePruningFreqInterval() {
	pruningFrequency := m.config.DockerPruningFrequency
	runImmediate := m.shouldRunImmediateFrequencyPrune(pruningFrequency)

	// Cancel the old frequency worker goroutine first.
	if m.freqCancel != nil {
		m.freqCancel()
	}
	if m.frequencyTicker != nil {
		m.frequencyTicker.Stop()
		m.frequencyTicker = nil
	}

	// Create a fresh context for the new worker.
	m.freqCtx, m.freqCancel = context.WithCancel(context.Background())

	if pruningFrequency > 0 {
		duration := time.Duration(pruningFrequency) * time.Hour
		m.frequencyTicker = time.NewTicker(duration)
		go m.frequencyPruningWorker()
		if runImmediate {
			go m.triggerPruneOnFrequency()
		}
		logging.LogInfo(moduleName, fmt.Sprintf("Docker pruning frequency updated to: %d hours", pruningFrequency))
	} else {
		logging.LogInfo(moduleName, "Docker pruning frequency set to 0 - frequency-based pruning disabled")
	}
	m.setLastAppliedPruningFrequency(pruningFrequency)
}

func (m *Manager) shouldRunImmediateFrequencyPrune(newFrequency int64) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if newFrequency <= 0 {
		return false
	}
	return m.lastAppliedPruningFrequency != newFrequency
}

func (m *Manager) setLastAppliedPruningFrequency(freq int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastAppliedPruningFrequency = freq
}

// GetName returns the module name
func (m *Manager) GetName() string {
	return moduleName
}

// GetModuleIndex returns the module index (Docker Pruning Manager doesn't have a specific index)
func (m *Manager) GetModuleIndex() int {
	return -1 // Docker Pruning Manager is not tracked in status
}
