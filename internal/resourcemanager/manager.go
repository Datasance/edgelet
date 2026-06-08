package resourcemanager

import (
	"context"
	"sync"
	"time"

	"github.com/eclipse-iofog/edgelet/internal/config"
	"github.com/eclipse-iofog/edgelet/internal/fieldagent"
	"github.com/eclipse-iofog/edgelet/internal/utils"
	"github.com/eclipse-iofog/edgelet/internal/utils/logging"
)

const (
	moduleName = "Resource Manager"
)

// Manager handles resource management tasks
type Manager struct {
	config       *config.Config
	fieldAgent   *fieldagent.FieldAgent
	ctx          context.Context
	cancel       context.CancelFunc
	workerCtx    context.Context
	workerCancel context.CancelFunc
	wg           sync.WaitGroup
	mu           sync.RWMutex
}

var (
	instance *Manager
	once     sync.Once
)

// GetInstance returns the singleton ResourceManager instance
func GetInstance() *Manager {
	once.Do(func() {
		instance = &Manager{
			config: config.GetInstance(),
		}
	})
	return instance
}

// Start starts the ResourceManager
func (rm *Manager) Start() error {
	logging.LogDebug(moduleName, "Starting Resource Manager")

	rm.fieldAgent = fieldagent.GetInstance()

	// Create context for cancellation
	rm.ctx, rm.cancel = context.WithCancel(context.Background())

	// Start background worker
	rm.startWorker()

	logging.LogDebug(moduleName, "Resource Manager started")
	return nil
}

// startWorker starts the usage data worker
func (rm *Manager) startWorker() {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	// Cancel existing worker if any
	if rm.workerCancel != nil {
		rm.workerCancel()
	}

	// Create new worker context
	rm.workerCtx, rm.workerCancel = context.WithCancel(rm.ctx)

	rm.wg.Add(1)
	go rm.runUsageDataWorker(rm.workerCtx)
}

// Stop stops the ResourceManager
func (rm *Manager) Stop() error {
	logging.LogDebug(moduleName, "Stopping Resource Manager")

	if rm.cancel != nil {
		rm.cancel()
	}

	// Wait for all workers to finish
	rm.wg.Wait()

	logging.LogDebug(moduleName, "Resource Manager stopped")
	return nil
}

// GetName returns the module name
func (rm *Manager) GetName() string {
	return moduleName
}

// GetModuleIndex returns the module index
func (rm *Manager) GetModuleIndex() int {
	return utils.ResourceManager
}

// InstanceConfigUpdated updates the ResourceManager when configuration changes
func (rm *Manager) InstanceConfigUpdated() {
	logging.LogDebug(moduleName, "Updating Resource Manager configuration")
	rm.startWorker()
}

// runUsageDataWorker periodically gets usage data and sends HW/USB info to controller
func (rm *Manager) runUsageDataWorker(ctx context.Context) {
	defer rm.wg.Done()

	cfg := rm.config
	ticker := time.NewTicker(time.Duration(cfg.DeviceScanFrequency) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			logging.LogDebug(moduleName, "Start getting usage data")

			// Send USB info from HAL to controller
			rm.fieldAgent.SendUSBInfoFromHalToController()

			// Send HW info from HAL to controller
			rm.fieldAgent.SendHWInfoFromHalToController()

			logging.LogDebug(moduleName, "Finished getting usage data")
		}
	}
}
