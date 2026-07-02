package processmanager

import (
	"github.com/eclipse-iofog/edgelet/internal/models"
)

// syncMicroserviceStatusToReporter stores runtime status for Pot reporting.
// RUNNING clears prior errorMessage explicitly so the controller receives errorMessage:"".
func syncMicroserviceStatusToReporter(pmStatus *models.ProcessManagerStatus, uuid string, status *models.MicroserviceStatus) {
	if pmStatus == nil || uuid == "" {
		return
	}
	pmStatus.SetMicroservicesStatus(uuid, status)
	if status != nil && status.Status == models.MicroserviceStateRunning {
		pmStatus.SetMicroservicesStatusErrorMessage(uuid, "")
	}
}
