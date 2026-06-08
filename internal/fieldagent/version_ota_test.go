package fieldagent

import (
	"testing"

	"github.com/eclipse-iofog/edgelet/internal/config"
	"github.com/eclipse-iofog/edgelet/internal/models"
	"github.com/eclipse-iofog/edgelet/internal/statusreporter"
)

func TestChangeVersionReadinessStatusPayload(t *testing.T) {
	sr := statusreporter.GetInstance()
	sr.UpdateFieldAgentStatus(func(s *models.FieldAgentStatus) {
		s.ReadyToUpgrade = true
		s.ReadyToRollback = false
	})
	t.Cleanup(func() {
		sr.UpdateFieldAgentStatus(func(s *models.FieldAgentStatus) {
			s.ReadyToUpgrade = false
			s.ReadyToRollback = false
		})
	})

	fa := &FieldAgent{config: config.GetInstance(), state: NewState()}
	status := fa.getFogStatus()

	readyToUpgrade, ok := status["isReadyToUpgrade"].(bool)
	if !ok || !readyToUpgrade {
		t.Fatalf("expected isReadyToUpgrade=true, got %v", status["isReadyToUpgrade"])
	}
	readyToRollback, ok := status["isReadyToRollback"].(bool)
	if !ok || readyToRollback {
		t.Fatalf("expected isReadyToRollback=false, got %v", status["isReadyToRollback"])
	}
}

func TestChangeVersionScanUpdatesFieldAgentStatus(t *testing.T) {
	sr := statusreporter.GetInstance()
	sr.UpdateFieldAgentStatus(func(s *models.FieldAgentStatus) {
		s.ReadyToUpgrade = true
		s.ReadyToRollback = true
	})
	t.Cleanup(func() {
		sr.UpdateFieldAgentStatus(func(s *models.FieldAgentStatus) {
			s.ReadyToUpgrade = false
			s.ReadyToRollback = false
		})
	})

	fa := &FieldAgent{config: config.GetInstance(), state: NewState()}
	fa.scanVersionReadiness()

	got := sr.GetFieldAgentStatus()
	if got.ReadyToUpgrade || got.ReadyToRollback {
		t.Fatalf("scanVersionReadiness should reflect handler defaults on dev host, got upgrade=%v rollback=%v",
			got.ReadyToUpgrade, got.ReadyToRollback)
	}
}
