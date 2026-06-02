package supervisor

import (
	"context"
	"fmt"
	"strings"

	"github.com/datasance/edgelet/internal/config"
	"github.com/datasance/edgelet/internal/constants"
	"github.com/datasance/edgelet/internal/fieldagent"
	"github.com/datasance/edgelet/internal/models"
	"github.com/datasance/edgelet/internal/processmanager"
	"github.com/datasance/edgelet/internal/runtime"
	"github.com/datasance/edgelet/internal/utils"
	"github.com/datasance/edgelet/internal/utils/logging"
	"github.com/datasance/edgelet/pkg/engine"
)

// reloadEngineContext carries per-reload metadata for warm reload revert.
type reloadEngineContext struct {
	priorDockerURL string
}

func (s *Supervisor) handleEngineConfigReload(reloadCtx *reloadEngineContext) error {
	cfg := config.GetInstance()
	rs := runtime.GetState()
	startupEngine := rs.StartupEngine()
	priorURL := reloadCtx.priorDockerURL

	changeClass := config.ClassifyEngineConfigChange(
		startupEngine,
		cfg.ContainerEngine,
		startupEngineURL(startupEngine, priorURL),
		startupEngineURL(cfg.ContainerEngine, cfg.DockerURL),
		nil,
	)

	switch changeClass {
	case config.ChangeClassCold:
		return s.handleColdEngineChange()
	case config.ChangeClassWarm:
		return s.handleWarmDockerURLReload(reloadCtx)
	default:
		return nil
	}
}

func startupEngineURL(engineName, dockerURL string) string {
	if strings.EqualFold(strings.TrimSpace(engineName), constants.EngineEdgelet) {
		return constants.EdgeletEngineDockerURL()
	}
	return strings.TrimSpace(dockerURL)
}

func (s *Supervisor) handleColdEngineChange() error {
	logging.LogWarn(moduleName, "containerEngine change requires service restart; quiescing reconcile and cleaning up runtime state")

	processmanager.SetQuiesced(true)
	runtime.GetState().SetPendingRestart(true)

	ctx := context.Background()
	if s.ctx != nil {
		ctx = s.ctx
	}
	if s.processManager != nil {
		if err := s.processManager.CleanupForEngineSwitch(ctx); err != nil {
			logging.LogError(moduleName, "Engine switch cleanup failed", err)
			return err
		}
	}

	s.statusReporter.UpdateSupervisorStatus(func(status *models.SupervisorStatus) {
		status.SetModuleStatus(utils.ProcessManager, models.ModuleStatusWarning)
	})
	return nil
}

func (s *Supervisor) handleWarmDockerURLReload(reloadCtx *reloadEngineContext) error {
	cfg := config.GetInstance()
	if cfg.ContainerEngine != constants.EngineDocker && cfg.ContainerEngine != constants.EnginePodman {
		return nil
	}

	engConfig := engine.EngineConfig{
		SocketURL:  cfg.DockerURL,
		APIVersion: cfg.DockerAPIVersion,
		LogDir:     cfg.LogDiskDirectory + "containers",
	}

	newEng, err := initExternalEngineAttempt(cfg.ContainerEngine, engConfig)
	if err != nil {
		logging.LogError(moduleName, "Warm dockerUrl reload failed; reverting config", err)
		if revertErr := s.revertWarmReload(reloadCtx); revertErr != nil {
			logging.LogError(moduleName, "Failed to revert config after warm reload failure", revertErr)
		}
		return fmt.Errorf("dockerUrl reconnect failed: %w", err)
	}

	if err := s.swapContainerEngine(newEng, cfg.ContainerEngine); err != nil {
		logging.LogError(moduleName, "Failed to swap engine after warm reload", err)
		if revertErr := s.revertWarmReload(reloadCtx); revertErr != nil {
			logging.LogError(moduleName, "Failed to revert config after engine swap failure", revertErr)
		}
		return err
	}

	logging.LogInfo(moduleName, fmt.Sprintf("Warm %s dockerUrl reload succeeded", cfg.ContainerEngine))
	return nil
}

func (s *Supervisor) revertWarmReload(reloadCtx *reloadEngineContext) error {
	if reloadCtx == nil || reloadCtx.priorDockerURL == "" {
		return fmt.Errorf("missing warm reload prior dockerUrl")
	}
	return config.GetInstance().RevertDockerURL(reloadCtx.priorDockerURL)
}

// CaptureReloadEngineContext snapshots dockerUrl before config reload for warm revert.
func (s *Supervisor) CaptureReloadEngineContext() *reloadEngineContext {
	return s.captureReloadEngineContext()
}

func (s *Supervisor) captureReloadEngineContext() *reloadEngineContext {
	cfg := config.GetInstance()
	priorURL := cfg.DockerURL
	if snap, ok := cfg.ConsumeReloadPriorDockerURL(); ok {
		priorURL = snap
	}
	return &reloadEngineContext{
		priorDockerURL: priorURL,
	}
}

func (s *Supervisor) swapContainerEngine(eng engine.ContainerEngine, engineType string) error {
	s.engineWireMu.Lock()
	defer s.engineWireMu.Unlock()

	if s.processManager == nil {
		return fmt.Errorf("process manager not initialized")
	}
	s.processManager.SetEngine(eng, engineType)
	s.containerEngine = eng
	runtime.GetState().SetEngineReady(true)

	if s.dockerPruningManager != nil {
		s.dockerPruningManager.SetEngine(eng)
	}
	fieldagent.GetLogSessionManager().SetEngine(eng)
	return nil
}

func (s *Supervisor) liveExternalEngineConfig() engine.EngineConfig {
	cfg := config.GetInstance()
	return engine.EngineConfig{
		SocketURL:  cfg.DockerURL,
		APIVersion: cfg.DockerAPIVersion,
		LogDir:     cfg.LogDiskDirectory + "containers",
	}
}

// ReloadConfigWithContext notifies modules after config reload with engine lifecycle handling.
func (s *Supervisor) ReloadConfigWithContext(reloadCtx *reloadEngineContext) error {
	if reloadCtx == nil {
		reloadCtx = s.captureReloadEngineContext()
	}

	if err := s.handleEngineConfigReload(reloadCtx); err != nil {
		return err
	}
	return s.reloadHotConfig()
}
