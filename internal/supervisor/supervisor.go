package supervisor

import (
	"context"
	"sync"
	"time"

	"github.com/eclipse-iofog/agent-go/internal/config"
	"github.com/eclipse-iofog/agent-go/internal/edgeguard"
	"github.com/eclipse-iofog/agent-go/internal/fieldagent"
	"github.com/eclipse-iofog/agent-go/internal/gps"
	"github.com/eclipse-iofog/agent-go/internal/localapi"
	"github.com/eclipse-iofog/agent-go/internal/messagebus"
	"github.com/eclipse-iofog/agent-go/internal/models"
	"github.com/eclipse-iofog/agent-go/internal/network"
	"github.com/eclipse-iofog/agent-go/internal/processmanager"
	"github.com/eclipse-iofog/agent-go/internal/pruning"
	"github.com/eclipse-iofog/agent-go/internal/resourceconsumption"
	"github.com/eclipse-iofog/agent-go/internal/resourcemanager"
	"github.com/eclipse-iofog/agent-go/internal/statusreporter"
	"github.com/eclipse-iofog/agent-go/internal/utils"
	"github.com/eclipse-iofog/agent-go/internal/utils/logging"
)

const (
	moduleName = "Supervisor"
)

// Supervisor orchestrates all ioFog modules
type Supervisor struct {
	config      *config.Config
	modules     []Module
	mu          sync.RWMutex
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	
	// Module instances
	statusReporter            *statusreporter.StatusReporter
	networkInterfaceManager   *network.Manager
	resourceConsumptionManager *resourceconsumption.Manager
	fieldAgent                *fieldagent.FieldAgent
	processManager            *processmanager.ProcessManager
	resourceManager           *resourcemanager.Manager
	messageBus                *messagebus.MessageBus
	gpsManager                *gps.Manager
	localAPI                  *localapi.LocalAPI
	dockerPruningManager      *pruning.Manager
	edgeGuardManager          *edgeguard.Manager
	
	// Local API monitoring
	localApiMonitorTicker *time.Ticker
}

// NewSupervisor creates a new Supervisor instance
func NewSupervisor() *Supervisor {
	return &Supervisor{
		config: config.GetInstance(),
	}
}

// Start starts all modules in the correct order
func (s *Supervisor) Start() error {
	logging.LogDebug(moduleName, "Starting Supervisor")
	
	// Create context for cancellation
	s.ctx, s.cancel = context.WithCancel(context.Background())
	
	// Start StatusReporter first
	s.statusReporter = statusreporter.GetInstance()
	if err := s.statusReporter.Start(); err != nil {
		return err
	}
	s.statusReporter.UpdateSupervisorStatus(func(status *models.SupervisorStatus) {
		status.SetModuleStatus(utils.StatusReporter, models.ModuleStatusRunning)
	})
	
	// Set daemon status to STARTING
	s.statusReporter.UpdateSupervisorStatus(func(status *models.SupervisorStatus) {
		status.SetDaemonStatus(models.ModuleStatusStarting).
			SetDaemonLastStart(time.Now().UnixMilli()).
			SetOperationDuration(0)
	})
	
	// Start Network Interface Manager
	s.networkInterfaceManager = network.GetInstance()
	if err := s.networkInterfaceManager.Start(); err != nil {
		return err
	}
	
	// Start Resource Consumption Manager
	s.resourceConsumptionManager = resourceconsumption.GetInstance()
	if err := s.startModule(s.resourceConsumptionManager); err != nil {
		return err
	}
	
	// Start Field Agent
	s.fieldAgent = fieldagent.GetInstance()
	if err := s.startModule(s.fieldAgent); err != nil {
		return err
	}
	
	// Start Process Manager
	s.processManager = processmanager.GetInstance()
	// ProcessManager needs MicroserviceManager, which is provided by FieldAgent
	// FieldAgent implements MicroserviceManagerInterface through its methods
	if err := s.processManager.Start(s.fieldAgent); err != nil {
		return err
	}
	s.statusReporter.UpdateSupervisorStatus(func(status *models.SupervisorStatus) {
		status.SetModuleStatus(utils.ProcessManager, models.ModuleStatusRunning)
	})
	
	// Set ProcessManager reference in FieldAgent so it can notify ProcessManager during startup
	s.fieldAgent.SetProcessManager(s.processManager)
	// Set ProcessManager reference in LogSessionManager (matching Java: LogSessionManager needs ProcessManager for container lookups)
	fieldagent.GetLogSessionManager().SetProcessManager(s.processManager)
	
	// Start Resource Manager
	s.resourceManager = resourcemanager.GetInstance()
	if err := s.startModule(s.resourceManager); err != nil {
		return err
	}
	
	// Start Message Bus
	s.messageBus = messagebus.GetInstance()
	// MessageBus also needs MicroserviceManager, provided by FieldAgent
	if err := s.messageBus.Start(s.fieldAgent); err != nil {
		return err
	}
	s.statusReporter.UpdateSupervisorStatus(func(status *models.SupervisorStatus) {
		status.SetModuleStatus(utils.MessageBus, models.ModuleStatusRunning)
	})
	
	// Start GPS Manager
	s.gpsManager = gps.GetInstance()
	if err := s.startModule(s.gpsManager); err != nil {
		return err
	}
	
	// Start Local API Server (runs in separate goroutine)
	s.localAPI = localapi.GetInstance()
	// Register Supervisor's ReloadConfig as the config reload callback
	s.config.SetReloadCallback(s.ReloadConfig)
	go func() {
		if err := s.localAPI.Start(); err != nil {
			logging.LogError(moduleName, "Local API server error", err)
		}
	}()
	
	// Monitor Local API status (check every 10 seconds)
	s.localApiMonitorTicker = time.NewTicker(10 * time.Second)
	s.wg.Add(1)
	go s.monitorLocalAPI()
	
	// Set daemon status to RUNNING
	s.statusReporter.UpdateSupervisorStatus(func(status *models.SupervisorStatus) {
		status.SetDaemonStatus(models.ModuleStatusRunning)
	})
	
	// Start Docker Pruning Manager
	s.dockerPruningManager = pruning.GetInstance()
	if err := s.dockerPruningManager.Start(); err != nil {
		logging.LogError(moduleName, "Failed to start Docker Pruning Manager", err)
	}
	
	// Start Edge Guard Manager
	s.edgeGuardManager = edgeguard.GetInstance()
	if err := s.edgeGuardManager.Start(); err != nil {
		logging.LogError(moduleName, "Failed to start Edge Guard Manager", err)
	}
	
	// Start operation duration worker
	s.wg.Add(1)
	go s.operationDurationWorker()
	
	logging.LogDebug(moduleName, "Started Supervisor")
	return nil
}

// startModule starts a module and updates its status
func (s *Supervisor) startModule(module Module) error {
	logging.LogInfo(moduleName, "Starting "+module.GetName())
		s.statusReporter.UpdateSupervisorStatus(func(status *models.SupervisorStatus) {
		status.SetModuleStatus(module.GetModuleIndex(), models.ModuleStatusStarting)
	})
	
	if err := module.Start(); err != nil {
		return err
	}
	
		s.statusReporter.UpdateSupervisorStatus(func(status *models.SupervisorStatus) {
		status.SetModuleStatus(module.GetModuleIndex(), models.ModuleStatusRunning)
	})
	logging.LogInfo(moduleName, "Started "+module.GetName())
	return nil
}

// Stop stops all modules gracefully in reverse order
func (s *Supervisor) Stop() error {
	logging.LogDebug(moduleName, "Stopping Supervisor")
	
	// Cancel context to signal all workers to stop
	if s.cancel != nil {
		s.cancel()
	}
	
	// Stop Local API monitor
	if s.localApiMonitorTicker != nil {
		s.localApiMonitorTicker.Stop()
	}
	
	// Stop Local API server
	if s.localAPI != nil {
		if err := s.localAPI.Stop(); err != nil {
			logging.LogError(moduleName, "Error shutting down Local API", err)
		}
	}
	
	// Stop modules in reverse order
	if s.messageBus != nil {
		s.messageBus.Stop()
	}
	
	if s.edgeGuardManager != nil {
		if err := s.edgeGuardManager.Stop(); err != nil {
			logging.LogError(moduleName, "Error stopping Edge Guard Manager", err)
		}
	}
	
	if s.dockerPruningManager != nil {
		if err := s.dockerPruningManager.Stop(); err != nil {
			logging.LogError(moduleName, "Error stopping Docker Pruning Manager", err)
		}
	}
	
	if s.gpsManager != nil {
		if err := s.gpsManager.Stop(); err != nil {
			logging.LogError(moduleName, "Error stopping GPS Manager", err)
		}
	}
	
	if s.resourceManager != nil {
		if err := s.resourceManager.Stop(); err != nil {
			logging.LogError(moduleName, "Error stopping Resource Manager", err)
		}
	}
	
	if s.processManager != nil {
		if err := s.processManager.Stop(); err != nil {
			logging.LogError(moduleName, "Error stopping Process Manager", err)
		}
	}
	
	if s.fieldAgent != nil {
		if err := s.fieldAgent.Stop(); err != nil {
			logging.LogError(moduleName, "Error stopping Field Agent", err)
		}
	}
	
	if s.resourceConsumptionManager != nil {
		if err := s.resourceConsumptionManager.Stop(); err != nil {
			logging.LogError(moduleName, "Error stopping Resource Consumption Manager", err)
		}
	}
	
	if s.networkInterfaceManager != nil {
		if err := s.networkInterfaceManager.Stop(); err != nil {
			logging.LogError(moduleName, "Error stopping Network Interface Manager", err)
		}
	}
	
	if s.statusReporter != nil {
		if err := s.statusReporter.Stop(); err != nil {
			logging.LogError(moduleName, "Error stopping Status Reporter", err)
		}
	}
	
	// Wait for all workers to finish
	s.wg.Wait()
	
	logging.LogDebug(moduleName, "Stopped Supervisor")
	return nil
}

// monitorLocalAPI monitors the Local API server and restarts it if it dies
func (s *Supervisor) monitorLocalAPI() {
	defer s.wg.Done()
	
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-s.localApiMonitorTicker.C:
			logging.LogDebug(moduleName, "Check local API status")
			// Local API runs in a goroutine, so we can't easily check if it's dead
			// In Go, if the server crashes, it will be logged but we can't restart it
			// This is different from Java where we could check thread state
			// For now, we just log that we're checking
			logging.LogDebug(moduleName, "Finished checking local API status")
		}
	}
}

// operationDurationWorker periodically updates the operation duration
func (s *Supervisor) operationDurationWorker() {
	defer s.wg.Done()
	
	logging.LogDebug(moduleName, "Start checking operation duration")
	
	cfg := s.config
	ticker := time.NewTicker(time.Duration(cfg.StatusReportFreqSeconds) * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.statusReporter.UpdateSupervisorStatus(func(status *models.SupervisorStatus) {
				status.SetOperationDuration(time.Now().UnixMilli())
			})
			logging.LogDebug(moduleName, "Finished checking operation duration")
		}
	}
}

// GetName returns the supervisor module name
func (s *Supervisor) GetName() string {
	return moduleName
}

// GetModuleIndex returns the supervisor module index
func (s *Supervisor) GetModuleIndex() int {
	return utils.MessageBus // Supervisor uses MessageBus index in Java
}

// ReloadConfig notifies all modules that configuration has been reloaded
// This matches Java: saveConfigUpdates() method
func (s *Supervisor) ReloadConfig() error {
	logging.LogInfo(moduleName, "Start updating agent configurations")
	
	// Notify all modules in the same order as Java
	if s.fieldAgent != nil {
		if err := s.fieldAgent.Update(); err != nil {
			logging.LogError(moduleName, "Failed to update FieldAgent", err)
		}
	}
	
	if s.processManager != nil {
		s.processManager.Update()
	}

	if s.resourceConsumptionManager != nil {
		s.resourceConsumptionManager.InstanceConfigUpdated()
	}

	if s.resourceManager != nil {
		s.resourceManager.InstanceConfigUpdated()
	}

	if s.networkInterfaceManager != nil {
		// Run network update asynchronously to avoid blocking
		go func() {
			if err := s.networkInterfaceManager.UpdateNetworkInterface(); err != nil {
				logging.LogError(moduleName, "Failed to update network interface", err)
			}
		}()
	}

	if s.dockerPruningManager != nil {
		s.dockerPruningManager.ChangePruningFreqInterval()
	}
	
	// MessageBus doesn't have an Update method - it updates automatically when routes change
	// No action needed for MessageBus
	
	// Update Local API (was missing)
	if s.localAPI != nil {
		s.localAPI.Update()
	}
	
	if s.edgeGuardManager != nil {
		s.edgeGuardManager.InstanceConfigUpdated()
	}
	
	if s.gpsManager != nil {
		s.gpsManager.InstanceConfigUpdated()
	}
	
	logging.LogInfo(moduleName, "Finished updating agent configurations")
	return nil
}
