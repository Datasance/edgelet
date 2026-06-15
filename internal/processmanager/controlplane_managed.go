package processmanager

import (
	"strings"

	"github.com/eclipse-iofog/edgelet/internal/models"
	"github.com/eclipse-iofog/edgelet/internal/store"
)

func (pm *ProcessManager) isControlPlaneManagedMicroservice(ms *models.Microservice) bool {
	if ms == nil {
		return false
	}
	cp, found, err := store.GetInstance().GetSystemControlPlane()
	if err != nil || !found || cp == nil {
		return false
	}
	cpUUID := strings.TrimSpace(cp.ControllerUUID)
	if cpUUID == "" {
		return false
	}
	msUUID := strings.TrimSpace(ms.MicroserviceUUID)
	return msUUID == cpUUID
}
