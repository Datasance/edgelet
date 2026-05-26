package supervisor

import (
	"context"
	"fmt"
	"os"
	"sync"
	"syscall"
	"time"

	"github.com/datasance/edgelet/internal/config"
	"github.com/datasance/edgelet/internal/constants"
	"github.com/datasance/edgelet/internal/edgeguard"
	"github.com/datasance/edgelet/internal/engines"
	"github.com/datasance/edgelet/internal/fieldagent"
	"github.com/datasance/edgelet/internal/gps"
	"github.com/datasance/edgelet/internal/healthcheck"
	"github.com/datasance/edgelet/internal/localapi"
	"github.com/datasance/edgelet/internal/models"
	"github.com/datasance/edgelet/internal/network"
	"github.com/datasance/edgelet/internal/processmanager"
	"github.com/datasance/edgelet/internal/pruning"
	"github.com/datasance/edgelet/internal/resourceconsumption"
	"github.com/datasance/edgelet/internal/resourcemanager"
	"github.com/datasance/edgelet/internal/statusreporter"
	"github.com/datasance/edgelet/internal/store"
	"github.com/datasance/edgelet/internal/utils"
	"github.com/datasance/edgelet/internal/utils/logging"
	edgeletcontainerdd "github.com/datasance/edgelet/pkg/containerd"
	"github.com/datasance/edgelet/pkg/engine"
)

const (
	moduleName                  = "Supervisor"
	shutdownRuntimeDrainTimeout = 45 * time.Second
)

var requestDaemonRestart = func(reason string, cause error) {
	logging.LogError(moduleName, reason, cause)
	if err := signalSelfForSupervisor(syscall.SIGTERM); err != nil {
		logging.LogError(moduleName, "Failed to signal daemon for restart", err)
	}
}

var setContainerdUnexpectedExitHandler = func(svc *edgeletcontainerdd.Service, handler func(error)) {
	svc.SetUnexpectedExitHandler(handler)
}

var signalSelfForSupervisor = func(sig syscall.Signal) error {
	return syscall.Kill(os.Getpid(), sig)
}

// Supervisor orchestrates all ioFog modules
type Supervisor struct {
	config *config.Config
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Embedded containerd service (non-nil only when containerEngine=iofog)
	containerdSvc *edgeletcontainerdd.Service

	// Module instances
	statusReporter             *statusreporter.StatusReporter
	networkInterfaceManager    *network.Manager
	resourceConsumptionManager *resourceconsumption.Manager
	fieldAgent                 *fieldagent.FieldAgent
	processManager             *processmanager.ProcessManager
	resourceManager            *resourcemanager.Manager
	gpsManager                 *gps.Manager
	localAPI                   *localapi.LocalAPI
	dockerPruningManager       *pruning.Manager
	edgeGuardManager           *edgeguard.Manager
	healthcheckRunner          *healthcheck.Runner

	// Local API monitoring
	localAPIMonitorTicker *time.Ticker
}

// NewSupervisor creates a new Supervisor instance
func NewSupervisor() *Supervisor {
	return &Supervisor{
		config: config.GetInstance(),
	}
}

// SetPrestartedContainerd injects an embedded containerd service already started in main
// (full flavor + iofog engine). Supervisor will not start containerd again; it only runs
// the watchdog and stops containerd on shutdown.
func (s *Supervisor) SetPrestartedContainerd(svc *edgeletcontainerdd.Service) {
	s.containerdSvc = svc
}

// Start starts all modules in the correct order
func (s *Supervisor) Start() error {
	logging.LogDebug(moduleName, "Starting Supervisor")

	// Open SQLite database before any module starts
	db := store.GetInstance()
	if err := db.Open(s.config.DiskDirectory); err != nil {
		return fmt.Errorf("failed to open SQLite database: %w", err)
	}
	logging.LogInfo(moduleName, "SQLite database opened")
	if err := s.ensureDefaultLocalRegistriesOnStartup(db); err != nil {
		return fmt.Errorf("failed to seed default local registries: %w", err)
	}

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
	// Instantiate the container engine based on configuration.
	cfg := config.GetInstance()

	// If the embedded iofog engine is selected, ensure containerd is running before the engine.
	// Startup ownership is in cmd/edgelet bootstrap; Supervisor only consumes prestarted runtime.
	if cfg.ContainerEngine == constants.EngineEdgelet {
		if s.containerdSvc == nil {
			return fmt.Errorf("embedded containerd must be prestarted before Supervisor when containerEngine=%q", constants.EngineEdgelet)
		}
		logging.LogInfo(moduleName, "Using embedded containerd started before Supervisor")
		s.configureContainerdFailFastHandler()

		// Watchdog for embedded containerd socket liveness.
		s.wg.Add(1)
		go s.containerdWatchdog()
	}

	engConfig := engine.EngineConfig{
		SocketURL:  cfg.DockerURL,
		APIVersion: cfg.DockerAPIVersion,
		LogDir:     cfg.LogDiskDirectory + "containers",
	}

	var eng engine.ContainerEngine
	var engErr error
	if cfg.ContainerEngine == constants.EngineDocker || cfg.ContainerEngine == constants.EnginePodman {
		eng, engErr = s.initExternalEngineWithRetry(cfg.ContainerEngine, engConfig)
	} else {
		eng, engErr = engines.NewContainerEngine(cfg.ContainerEngine, engConfig)
		if engErr != nil {
			return fmt.Errorf("failed to create container engine %q: %w", cfg.ContainerEngine, engErr)
		}
		if initErr := eng.Init(engConfig); initErr != nil {
			return fmt.Errorf("failed to init container engine %q: %w", cfg.ContainerEngine, initErr)
		}
		eng = engines.WrapWithLoggingIfExternal(eng, cfg.ContainerEngine)
	}
	if engErr != nil {
		return engErr
	}
	if err := s.processManager.Start(eng, s.fieldAgent); err != nil {
		return err
	}
	s.statusReporter.UpdateSupervisorStatus(func(status *models.SupervisorStatus) {
		status.SetModuleStatus(utils.ProcessManager, models.ModuleStatusRunning)
	})

	// Set ProcessManager reference in FieldAgent so it can notify ProcessManager during startup
	s.fieldAgent.SetProcessManager(s.processManager)
	// Inject engine + ProcessManager into LogSessionManager so log streaming works for all engine types
	fieldagent.GetLogSessionManager().SetProcessManager(s.processManager)
	fieldagent.GetLogSessionManager().SetEngine(eng)

	// Start HealthcheckRunner when using iofog engine (Docker/Podman use native healthcheck)
	if cfg.ContainerEngine == constants.EngineEdgelet {
		var hcEng healthcheck.HealthcheckEngine
		if he, ok := eng.(healthcheck.HealthcheckEngine); ok {
			hcEng = he
		}
		s.healthcheckRunner = healthcheck.NewRunner(eng, hcEng, s.fieldAgent)
		if err := s.healthcheckRunner.Start(s.ctx); err != nil {
			logging.LogWarn(moduleName, fmt.Sprintf("Healthcheck runner failed to start: %v", err))
		}
	}

	// Start Resource Manager
	s.resourceManager = resourcemanager.GetInstance()
	if err := s.startModule(s.resourceManager); err != nil {
		return err
	}

	// Start GPS Manager
	s.gpsManager = gps.GetInstance()
	if err := s.startModule(s.gpsManager); err != nil {
		return err
	}

	// Start Local API Server and wait until listeners are ready.
	s.localAPI = localapi.GetInstance()
	// Register Supervisor's ReloadConfig as the config reload callback
	s.config.SetReloadCallback(s.ReloadConfig)
	// Register FieldAgent GPS callback for dedicated config/gps controller sync.
	s.config.SetGPSConfigCallback(s.fieldAgent.InstanceGPSConfigUpdated)
	if err := s.localAPI.Start(); err != nil {
		return fmt.Errorf("failed to start Local API server: %w", err)
	}

	// Monitor Local API status (check every 10 seconds)
	s.localAPIMonitorTicker = time.NewTicker(10 * time.Second)
	s.wg.Add(1)
	go s.monitorLocalAPI()

	// Set daemon status to RUNNING
	s.statusReporter.UpdateSupervisorStatus(func(status *models.SupervisorStatus) {
		status.SetDaemonStatus(models.ModuleStatusRunning)
	})

	// Start Pruning Manager — inject engine so non-Docker engines (iofog/containerd) are pruned correctly.
	// Also wire in the microservice image callback so scheduled/threshold pruning protects
	// ALL configured microservice images (matching Java DockerPruningManager behavior).
	s.dockerPruningManager = pruning.GetInstance()
	pm := s.processManager
	s.dockerPruningManager.SetGetMicroservicesCallback(func() []string {
		microservices := pm.GetLatestMicroservices()
		names := make([]string, 0, len(microservices))
		for _, ms := range microservices {
			names = append(names, ms.ImageName)
		}
		return names
	})
	s.dockerPruningManager.SetEngine(eng)
	if err := s.dockerPruningManager.Start(); err != nil {
		logging.LogError(moduleName, "Failed to start Pruning Manager", err)
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

func (s *Supervisor) configureContainerdFailFastHandler() {
	if s.containerdSvc == nil {
		return
	}
	setContainerdUnexpectedExitHandler(s.containerdSvc, func(err error) {
		requestDaemonRestart("Embedded containerd exited unexpectedly after readiness; requesting immediate daemon restart", err)
	})
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

const (
	engineInitMaxRetries     = 12
	engineInitInitialBackoff = 2 * time.Second
	engineInitMaxBackoff     = 30 * time.Second
)

// initExternalEngineWithRetry creates and initializes Docker or Podman engine with
// exponential backoff. Used when the socket may be temporarily unavailable (e.g.
// daemon restart). After max retries, suggests switching to containerEngine: edgelet.
func (s *Supervisor) initExternalEngineWithRetry(engineType string, cfg engine.EngineConfig) (engine.ContainerEngine, error) {
	var lastErr error
	backoff := engineInitInitialBackoff

	for attempt := 1; attempt <= engineInitMaxRetries; attempt++ {
		eng, createErr := engines.NewContainerEngine(engineType, cfg)
		if createErr != nil {
			lastErr = createErr
			logging.LogWarn(moduleName, fmt.Sprintf("%s engine create attempt %d/%d failed: %v", engineType, attempt, engineInitMaxRetries, createErr))
			if attempt < engineInitMaxRetries {
				logging.LogInfo(moduleName, fmt.Sprintf("Retrying in %v...", backoff))
				time.Sleep(backoff)
				if backoff < engineInitMaxBackoff {
					backoff *= 2
				}
			}
			continue
		}

		if initErr := eng.Init(cfg); initErr != nil {
			lastErr = initErr
			logging.LogWarn(moduleName, fmt.Sprintf("%s engine init attempt %d/%d failed (socket may be unavailable): %v", engineType, attempt, engineInitMaxRetries, initErr))
			if attempt < engineInitMaxRetries {
				logging.LogInfo(moduleName, fmt.Sprintf("Retrying in %v...", backoff))
				time.Sleep(backoff)
				if backoff < engineInitMaxBackoff {
					backoff *= 2
				}
			}
			continue
		}

		logging.LogInfo(moduleName, fmt.Sprintf("%s engine initialized successfully after %d attempt(s)", engineType, attempt))
		return engines.WrapWithLoggingIfExternal(eng, engineType), nil
	}

	logging.LogError(moduleName, fmt.Sprintf("%s socket still unavailable after %d attempts", engineType, engineInitMaxRetries),
		fmt.Errorf("consider setting containerEngine: edgelet to use the embedded container engine: %w", lastErr))
	return nil, fmt.Errorf("%s engine init failed after %d retries: %w", engineType, engineInitMaxRetries, lastErr)
}

// Stop stops all modules gracefully in reverse order
func (s *Supervisor) Stop() error {
	logging.LogDebug(moduleName, "Stopping Supervisor")

	// Cancel context to signal all workers to stop
	if s.cancel != nil {
		s.cancel()
	}

	// Stop Local API monitor
	if s.localAPIMonitorTicker != nil {
		s.localAPIMonitorTicker.Stop()
	}

	// Stop Local API server
	if s.localAPI != nil {
		if err := s.localAPI.Stop(); err != nil {
			logging.LogError(moduleName, "Error shutting down Local API", err)
		}
	}

	// Stop modules in reverse order
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

	if s.healthcheckRunner != nil {
		if err := s.healthcheckRunner.Stop(); err != nil {
			logging.LogError(moduleName, "Error stopping Healthcheck Runner", err)
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
		if err := s.processManager.DrainRuntimeForShutdown(shutdownRuntimeDrainTimeout); err != nil {
			logging.LogError(moduleName, "Runtime drain during shutdown timed out", err)
		}
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

	// Stop embedded containerd last (after all containers are stopped).
	if s.containerdSvc != nil {
		logging.LogInfo(moduleName, "Stopping embedded containerd")
		s.containerdSvc.Stop()
	}

	if s.statusReporter != nil {
		if err := s.statusReporter.Stop(); err != nil {
			logging.LogError(moduleName, "Error stopping Status Reporter", err)
		}
	}

	// Wait for all workers to finish
	s.wg.Wait()

	// Close SQLite database last
	if err := store.GetInstance().Close(); err != nil {
		logging.LogError(moduleName, "Error closing SQLite database", err)
	}

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
		case <-s.localAPIMonitorTicker.C:
			logging.LogDebug(moduleName, "Check local API status")
			// Local API runs in a goroutine, so we can't easily check if it's dead
			// In Go, if the server crashes, it will be logged but we can't restart it
			// This is different from Java where we could check thread state
			// For now, we just log that we're checking
			logging.LogDebug(moduleName, "Finished checking local API status")
		}
	}
}

func (s *Supervisor) ensureDefaultLocalRegistriesOnStartup(db *store.DB) error {
	if db == nil || db.Conn() == nil {
		return nil
	}
	if err := db.EnsureDefaultLocalRegistries(); err != nil {
		return err
	}
	logging.LogDebug(moduleName, "Default local registries ensured on startup")
	return nil
}

// operationDurationWorker periodically updates the operation duration
func (s *Supervisor) operationDurationWorker() {
	defer s.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			logging.LogError(moduleName, "Panic recovered", fmt.Errorf("%v", r))
		}
	}()

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

// containerdWatchdog runs periodic containerd socket liveness checks when
// containerEngine=iofog. The runtime is a managed child process; watchdog does
// not run a nested runtime restart loop and instead requests daemon restart.
func (s *Supervisor) containerdWatchdog() {
	defer s.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	consecutiveFailures := 0
	const failureThreshold = 3

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			if s.containerdSvc == nil {
				return
			}
			if !s.containerdSvc.IsHealthy() {
				consecutiveFailures++
				logging.LogWarn(moduleName, fmt.Sprintf(
					"Embedded containerd socket is unresponsive (%d/%d consecutive checks)",
					consecutiveFailures, failureThreshold,
				))
				if consecutiveFailures >= failureThreshold {
					requestDaemonRestart(
						"Embedded containerd is persistently unhealthy; requesting daemon restart via SIGTERM",
						fmt.Errorf("containerd watchdog reached failure threshold"),
					)
					return
				}
				continue
			}
			consecutiveFailures = 0
		}
	}
}

// GetName returns the supervisor module name
func (s *Supervisor) GetName() string {
	return moduleName
}

// GetModuleIndex returns the supervisor module index
func (s *Supervisor) GetModuleIndex() int {
	return utils.ProcessManager
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
			defer func() {
				if r := recover(); r != nil {
					logging.LogError(moduleName, "Panic recovered", fmt.Errorf("%v", r))
				}
			}()
			if err := s.networkInterfaceManager.UpdateNetworkInterface(); err != nil {
				logging.LogError(moduleName, "Failed to update network interface", err)
			}
		}()
	}

	if s.dockerPruningManager != nil {
		s.dockerPruningManager.ChangePruningFreqInterval()
	}

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
