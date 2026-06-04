package processmanager

import (
	"context"
	"fmt"
	"sync"

	"github.com/datasance/edgelet/internal/models"
	"github.com/datasance/edgelet/internal/statusreporter"
	"github.com/datasance/edgelet/internal/store"
	"github.com/datasance/edgelet/pkg/engine"
)

var (
	quiesceMu sync.RWMutex
	quiesced  bool
)

// SetQuiesced blocks or resumes reconcile scheduling.
func SetQuiesced(v bool) {
	quiesceMu.Lock()
	quiesced = v
	quiesceMu.Unlock()
}

// IsQuiesced reports whether reconcile is paused (e.g. pending engine restart).
func IsQuiesced() bool {
	quiesceMu.RLock()
	defer quiesceMu.RUnlock()
	return quiesced
}

// SetEngine swaps the active container engine without restarting the process manager.
func (pm *ProcessManager) SetEngine(eng engine.ContainerEngine, engineName string) {
	pm.engine = eng
	pm.engineName = engineName
	if pm.containerManager != nil {
		pm.containerManager.engine = eng
		pm.containerManager.engineName = engineName
	}
}

// CleanupForEngineSwitch stops/removes all managed workload containers and clears ephemeral DB state.
func (pm *ProcessManager) CleanupForEngineSwitch(ctx context.Context) error {
	if pm == nil {
		return fmt.Errorf("process manager is nil")
	}

	pm.logger.Info("Cleaning up microservice runtime state for container engine change")

	if pm.engine != nil && pm.containerManager != nil {
		if pm.microserviceManager != nil {
			for _, ms := range pm.microserviceManager.GetLatestMicroservices() {
				if ms == nil || ms.Delete {
					continue
				}
				_ = pm.containerManager.RemoveContainerByMicroserviceUUID(ctx, ms.MicroserviceUUID, false, false)
			}
		}
		if items, err := store.GetInstance().ListLocalWorkloads(); err == nil {
			for _, item := range items {
				if item == nil {
					continue
				}
				if item.ContainerID != "" {
					_ = pm.containerManager.RemoveContainerRuntimeForEngineSwitch(ctx, item.ContainerID)
					continue
				}
				if container, err := pm.containerManager.GetContainerForMicroservice(item.LocalUUID); err == nil && container != nil {
					_ = pm.containerManager.RemoveContainerRuntimeForEngineSwitch(ctx, container.ID)
				}
			}
		}
	}

	db := store.GetInstance()
	if err := db.ClearRuntimeContainerRefs(""); err != nil {
		return err
	}
	if err := db.ClearControllerMicroserviceRuntimeFields(); err != nil {
		return err
	}
	if err := db.ClearLocalWorkloadRuntimeFields(); err != nil {
		return err
	}

	statusreporter.GetInstance().UpdateProcessManagerStatus(func(s *models.ProcessManagerStatus) {
		if s == nil {
			return
		}
		s.ClearMicroserviceStatuses()
	})

	pm.notifyMonitorThread()
	return nil
}
