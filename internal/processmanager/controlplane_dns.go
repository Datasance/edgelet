package processmanager

import (
	"strings"

	"github.com/eclipse-iofog/edgelet/internal/dnsresolver"
	"github.com/eclipse-iofog/edgelet/internal/models"
)

func (pm *ProcessManager) syncControlPlaneDNS(item *models.ControlPlaneDeployment, active bool) {
	if item == nil {
		return
	}
	uuid := strings.TrimSpace(item.ControllerUUID)
	if uuid == "" {
		return
	}
	ip := ""
	if strings.TrimSpace(item.ContainerID) != "" && pm.engine != nil {
		if addr, err := pm.engine.GetContainerIPAddress(item.ContainerID); err == nil {
			ip = strings.TrimSpace(addr)
		}
	}
	rec := dnsresolver.WorkloadRecord{
		UUID:         uuid,
		Application:  strings.TrimSpace(item.Namespace),
		Name:         strings.TrimSpace(item.Name),
		Scope:        dnsresolver.ScopeLocal,
		IP:           ip,
		IsController: true,
		Active:       active,
	}
	dnsresolver.GetInstance().UpsertWorkload(rec)
}

func (pm *ProcessManager) removeControlPlaneDNS(controllerUUID string) {
	dnsresolver.GetInstance().RemoveWorkload(strings.TrimSpace(controllerUUID))
}
