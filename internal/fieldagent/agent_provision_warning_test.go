package fieldagent

import (
	"testing"

	"github.com/eclipse-iofog/edgelet/internal/models"
	"github.com/eclipse-iofog/edgelet/internal/statusreporter"
)

func TestClearSupervisorWarningAfterProvision(t *testing.T) {
	sr := statusreporter.GetInstance()
	sr.UpdateSupervisorStatus(func(status *models.SupervisorStatus) {
		status.SetDaemonStatus(models.ModuleStatusWarning)
		status.SetWarningMessage("HW signature changed")
	})
	t.Cleanup(func() {
		sr.UpdateSupervisorStatus(func(status *models.SupervisorStatus) {
			status.SetWarningMessage("")
			status.SetDaemonStatus(models.ModuleStatusRunning)
		})
	})

	clearSupervisorWarningAfterProvision()

	got := sr.GetSupervisorStatus()
	if got.WarningMessage != "" {
		t.Fatalf("expected empty warning message, got %q", got.WarningMessage)
	}
	if got.DaemonStatus != models.ModuleStatusRunning {
		t.Fatalf("expected daemon RUNNING, got %q", got.DaemonStatus)
	}
}
