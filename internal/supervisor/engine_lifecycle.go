package supervisor

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/eclipse-iofog/edgelet/internal/config"
	"github.com/eclipse-iofog/edgelet/internal/constants"
	"github.com/eclipse-iofog/edgelet/internal/fieldagent"
	"github.com/eclipse-iofog/edgelet/internal/models"
	"github.com/eclipse-iofog/edgelet/internal/processmanager"
	"github.com/eclipse-iofog/edgelet/internal/runtimestate"
	"github.com/eclipse-iofog/edgelet/internal/utils"
	"github.com/eclipse-iofog/edgelet/internal/utils/logging"
	"github.com/eclipse-iofog/edgelet/pkg/engine"
)

// reloadEngineContext carries per-reload metadata for warm reload revert.
type reloadEngineContext struct {
	priorContainerEngineURL string
}

func (s *Supervisor) handleEngineConfigReload(reloadCtx *reloadEngineContext) error {
	cfg := config.GetInstance()
	rs := runtimestate.GetState()
	startupEngine := rs.StartupEngine()
	priorURL := reloadCtx.priorContainerEngineURL

	changeClass := config.ClassifyEngineConfigChange(
		startupEngine,
		cfg.ContainerEngine,
		startupEngineURL(startupEngine, priorURL),
		startupEngineURL(cfg.ContainerEngine, cfg.ContainerEngineURL),
		nil,
	)

	switch changeClass {
	case config.ChangeClassCold:
		return s.handleColdEngineChange()
	case config.ChangeClassWarm:
		return s.handleWarmContainerEngineURLReload(reloadCtx)
	default:
		return nil
	}
}

func startupEngineURL(engineName, containerEngineURL string) string {
	if strings.EqualFold(strings.TrimSpace(engineName), constants.EngineEdgelet) {
		return constants.EdgeletEngineSocketURL()
	}
	return strings.TrimSpace(containerEngineURL)
}

func (s *Supervisor) handleColdEngineChange() error {
	if processmanager.IsQuiesced() && runtimestate.GetState().PendingRestart() {
		logging.LogDebug(moduleName, "cold engine switch already active; skipping duplicate cleanup")
		processmanager.SetQuiesced(true)
		return nil
	}

	logging.LogWarn(moduleName, "containerEngine change requires service restart; quiescing reconcile and cleaning up runtime state")

	processmanager.SetQuiesced(true)
	runtimestate.GetState().SetPendingRestart(true)

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

func (s *Supervisor) handleWarmContainerEngineURLReload(reloadCtx *reloadEngineContext) error {
	cfg := config.GetInstance()
	if cfg.ContainerEngine != constants.EngineDocker && cfg.ContainerEngine != constants.EnginePodman {
		return nil
	}

	engConfig := engine.EngineConfig{
		SocketURL:  cfg.ContainerEngineURL,
		APIVersion: cfg.DockerAPIVersion,
		LogDir:     cfg.LogDirectory + "containers",
	}

	newEng, err := initExternalEngineAttempt(cfg.ContainerEngine, engConfig)
	if err != nil {
		logging.LogError(moduleName, "Warm containerEngineUrl reload failed; reverting config", err)
		if revertErr := s.revertWarmReload(reloadCtx); revertErr != nil {
			logging.LogError(moduleName, "Failed to revert config after warm reload failure", revertErr)
		}
		return fmt.Errorf("containerEngineUrl reconnect failed: %w", err)
	}

	if err := s.swapContainerEngine(newEng, cfg.ContainerEngine); err != nil {
		logging.LogError(moduleName, "Failed to swap engine after warm reload", err)
		if revertErr := s.revertWarmReload(reloadCtx); revertErr != nil {
			logging.LogError(moduleName, "Failed to revert config after engine swap failure", revertErr)
		}
		return err
	}

	logging.LogInfo(moduleName, fmt.Sprintf("Warm %s containerEngineUrl reload succeeded", cfg.ContainerEngine))
	return nil
}

func (s *Supervisor) revertWarmReload(reloadCtx *reloadEngineContext) error {
	if reloadCtx == nil || reloadCtx.priorContainerEngineURL == "" {
		return errors.New("missing warm reload prior containerEngineUrl")
	}
	return config.GetInstance().RevertContainerEngineURL(reloadCtx.priorContainerEngineURL)
}

// CaptureReloadEngineContext snapshots containerEngineUrl before config reload for warm revert.
//
//nolint:revive // returns internal reload snapshot type by design
func (s *Supervisor) CaptureReloadEngineContext() *reloadEngineContext {
	return s.captureReloadEngineContext()
}

func (s *Supervisor) captureReloadEngineContext() *reloadEngineContext {
	cfg := config.GetInstance()
	priorURL := cfg.ContainerEngineURL
	if snap, ok := cfg.ConsumeReloadPriorContainerEngineURL(); ok {
		priorURL = snap
	}
	return &reloadEngineContext{
		priorContainerEngineURL: priorURL,
	}
}

func (s *Supervisor) swapContainerEngine(eng engine.ContainerEngine, engineType string) error {
	s.engineWireMu.Lock()
	defer s.engineWireMu.Unlock()

	if s.processManager == nil {
		return errors.New("process manager not initialized")
	}
	s.processManager.SetEngine(eng, engineType)
	s.containerEngine = eng
	runtimestate.GetState().SetEngineReady(true)
	processmanager.TryResumeReconcileAfterDataPlaneEngineReady()

	if s.dockerPruningManager != nil {
		s.dockerPruningManager.SetEngine(eng)
	}
	fieldagent.GetLogSessionManager().SetEngine(eng)
	return nil
}

func (s *Supervisor) liveExternalEngineConfig() engine.EngineConfig {
	cfg := config.GetInstance()
	return engine.EngineConfig{
		SocketURL:  cfg.ContainerEngineURL,
		APIVersion: cfg.DockerAPIVersion,
		LogDir:     cfg.LogDirectory + "containers",
	}
}

// ReloadConfigWithContext notifies modules after config reload with engine lifecycle handling.
func (s *Supervisor) ReloadConfigWithContext(reloadCtx *reloadEngineContext) error {
	s.reloadMu.Lock()
	defer s.reloadMu.Unlock()

	if reloadCtx == nil {
		reloadCtx = s.captureReloadEngineContext()
	}

	if err := s.handleEngineConfigReload(reloadCtx); err != nil {
		return err
	}
	return s.reloadHotConfig()
}
