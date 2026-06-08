package processmanager

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/eclipse-iofog/edgelet/internal/controlplane"
	"github.com/eclipse-iofog/edgelet/internal/models"
	"github.com/eclipse-iofog/edgelet/internal/store"
)

var ErrControlPlaneNotFound = errors.New("control plane deployment not found")

// SyncApplyControlPlaneDeployment persists and synchronously launches or recreates the controller container.
func (pm *ProcessManager) SyncApplyControlPlaneDeployment(item *models.ControlPlaneDeployment, progress LocalDeployProgressCallback) error {
	if item == nil {
		return errors.New("control plane deployment is nil")
	}
	if pm.containerManager == nil {
		return errors.New("process manager is not initialized")
	}

	item.NormalizeDefaults()
	nowSec := time.Now().Unix()
	item.DesiredState = "running"
	item.DeletedAt = nil
	item.LastStartAttemptAt = nowSec
	item.LastTransitionAt = nowSec

	container, contErr := pm.containerForControlPlane(item.ControllerUUID, item.ContainerID)
	if contErr != nil {
		return contErr
	}

	if container != nil {
		if err := pm.recreateControlPlaneDeploymentWithProgress(item, false, nowSec, progress); err != nil {
			return err
		}
	} else {
		pm.launchControlPlaneWithProgress(item, nowSec, progress)
	}

	got, found, err := store.GetInstance().GetSystemControlPlane()
	if err != nil {
		return err
	}
	if !found || got == nil {
		return errors.New("control plane deployment missing after apply")
	}
	if strings.EqualFold(strings.TrimSpace(got.RuntimeState), "failed") {
		if msg := strings.TrimSpace(got.LastError); msg != "" {
			return fmt.Errorf("%s", msg)
		}
		return errors.New("control plane launch failed")
	}
	return nil
}

// DeleteControlPlane stops the controller container, removes volumes, and deletes the singleton row.
func (pm *ProcessManager) DeleteControlPlane() error {
	item, found, err := store.GetInstance().GetSystemControlPlane()
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

	return store.GetInstance().DeleteSystemControlPlane()
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
