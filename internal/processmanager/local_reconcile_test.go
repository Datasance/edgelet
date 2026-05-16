package processmanager

import (
	"testing"

	"github.com/eclipse-iofog/agent/internal/models"
)

func TestBumpLocalFailureMarksStuckAfterThreshold(t *testing.T) {
	pm := &ProcessManager{}
	item := &models.LocalDeployedMicroservice{
		LocalUUID:    "local-1",
		RuntimeState: "failed",
		State:        "failed",
		FailureCount: localReconcileMaxFailures - 1,
		RestartCount: 2,
	}

	pm.bumpLocalFailure(item, nil, "failed")

	if item.FailureCount != localReconcileMaxFailures {
		t.Fatalf("expected failure_count=%d, got %d", localReconcileMaxFailures, item.FailureCount)
	}
	if item.RuntimeState != "stuck_in_restart" {
		t.Fatalf("expected runtime_state=stuck_in_restart, got %q", item.RuntimeState)
	}
	if item.State != "stuck_in_restart" {
		t.Fatalf("expected state=stuck_in_restart, got %q", item.State)
	}
	if item.RestartCount != 3 {
		t.Fatalf("expected restart_count increment, got %d", item.RestartCount)
	}
}
