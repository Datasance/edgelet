package runtimeapi

import (
	"errors"
	"fmt"
	"strings"

	"github.com/eclipse-iofog/edgelet/internal/models"
)

// ErrControlPlaneLifecycleBlocked indicates ms rm/stop/kill/start/restart on the controller UUID is forbidden.
type ErrControlPlaneLifecycleBlocked struct {
	Operation string
}

func (e *ErrControlPlaneLifecycleBlocked) Error() string {
	op := strings.TrimSpace(e.Operation)
	if op == "" {
		op = "mutation"
	}
	return fmt.Sprintf(
		"control plane microservice %s is not allowed; use DELETE /v1/system/controlplane or edgelet controlplane delete",
		op,
	)
}

func (f *Facade) controlPlaneDeploymentRow() (*models.ControlPlaneDeployment, bool) {
	if f == nil || f.db == nil || f.db.Conn() == nil {
		return nil, false
	}
	item, found, err := f.db.GetSystemControlPlane()
	if err != nil || !found || item == nil {
		return nil, false
	}
	if strings.EqualFold(strings.TrimSpace(item.DesiredState), "deleted") {
		return nil, false
	}
	return item, true
}

func (f *Facade) guardControlPlaneMicroserviceMutation(uuid, operation string) error {
	item, ok := f.controlPlaneDeploymentRow()
	if !ok {
		return nil
	}
	if strings.TrimSpace(item.ControllerUUID) != strings.TrimSpace(uuid) {
		return nil
	}
	return &ErrControlPlaneLifecycleBlocked{Operation: operation}
}

func controlPlaneRuntimeListEntry(item *models.ControlPlaneDeployment) map[string]any {
	if item == nil {
		return nil
	}
	item.NormalizeDefaults()
	state := strings.ToLower(strings.TrimSpace(item.RuntimeState))
	if state == "" {
		state = strings.ToLower(strings.TrimSpace(item.State))
	}
	return map[string]any{
		"uuid":        item.ControllerUUID,
		"name":        item.Name,
		"application": item.Namespace,
		"source":      "controlplane",
		"type":        "controlplane",
		"state":       state,
		"containerId": item.ContainerID,
		"image":       item.Image,
	}
}

// IsControlPlaneLifecycleBlocked reports whether err blocks control-plane ms lifecycle.
func IsControlPlaneLifecycleBlocked(err error) bool {
	var blocked *ErrControlPlaneLifecycleBlocked
	return errors.As(err, &blocked)
}
