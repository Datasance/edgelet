package processmanager

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/datasance/edgelet/internal/controlplane"
	"github.com/datasance/edgelet/internal/models"
	"github.com/datasance/edgelet/internal/store"
)

var ErrControlPlaneNotFound = errors.New("control plane deployment not found")

// SyncApplyControlPlaneDeployment persists and synchronously launches or recreates the controller container.
func (pm *ProcessManager) SyncApplyControlPlaneDeployment(item *models.ControlPlaneDeployment, progress LocalDeployProgressCallback) error {
	if item == nil {
		return fmt.Errorf("control plane deployment is nil")
	}
	if pm.containerManager == nil {
		return fmt.Errorf("process manager is not initialized")
	}

	item.NormalizeDefaults()
	now := time.Now().Unix()
	item.DesiredState = "running"
	item.DeletedAt = nil
	item.LastStartAttemptAt = now
	item.LastTransitionAt = now

	container, contErr := pm.containerForControlPlane(item.ControllerUUID, item.ContainerID)
	if contErr != nil {
		return contErr
	}

	if container != nil {
		if err := pm.recreateControlPlaneDeploymentWithProgress(item, false, now, progress); err != nil {
			return err
		}
	} else {
		pm.launchControlPlaneWithProgress(item, now, progress)
	}

	got, found, err := store.GetInstance().GetControlPlaneDeployment()
	if err != nil {
		return err
	}
	if !found || got == nil {
		return fmt.Errorf("control plane deployment missing after apply")
	}
	if strings.EqualFold(strings.TrimSpace(got.RuntimeState), "failed") {
		if msg := strings.TrimSpace(got.LastError); msg != "" {
			return fmt.Errorf("%s", msg)
		}
		return fmt.Errorf("control plane launch failed")
	}
	return nil
}

// DeleteControlPlane stops the controller container, removes volumes, and deletes the singleton row.
func (pm *ProcessManager) DeleteControlPlane() error {
	item, found, err := store.GetInstance().GetControlPlaneDeployment()
	if err != nil {
		return err
	}
	if !found || item == nil {
		return ErrControlPlaneNotFound
	}

	container, contErr := pm.containerForControlPlane(item.ControllerUUID, item.ContainerID)
	if contErr != nil {
		return contErr
	}
	if container != nil && pm.containerManager != nil {
		if err := pm.removeLocalContainerByID(container.ID); err != nil {
			return fmt.Errorf("failed to remove control plane container: %w", err)
		}
	}

	pm.removeControlPlaneDNS(item.ControllerUUID)

	pm.removeControlPlaneVolumes()

	if err := store.GetInstance().DeleteControlPlaneDeployment(); err != nil {
		return err
	}
	return nil
}

func (pm *ProcessManager) removeControlPlaneVolumes() {
	if pm.engine == nil {
		return
	}
	ctx := context.Background()
	for _, name := range []string{controlplane.VolumeDBName, controlplane.VolumeLogName} {
		if err := pm.engine.RemoveNamedVolume(ctx, name); err != nil {
			pm.logger.Warnf("control plane volume cleanup failed name=%s err=%v", name, err)
		}
	}
}
