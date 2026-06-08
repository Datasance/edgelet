package processmanager

import (
	"context"
	"testing"
	"time"

	"github.com/eclipse-iofog/edgelet/internal/models"
	"github.com/eclipse-iofog/edgelet/internal/runtimeops"
	"github.com/eclipse-iofog/edgelet/internal/statusreporter"
	"github.com/eclipse-iofog/edgelet/internal/utils/logging"
)

func TestShouldEmitReconcileCycle_IdleThrottled(t *testing.T) {
	stats := &reconcileCycleStats{}
	if shouldEmitReconcileCycle(stats, 1, 60) {
		t.Fatal("tick 1 should not emit idle cycle")
	}
	if !shouldEmitReconcileCycle(stats, 60, 60) {
		t.Fatal("tick 60 should emit heartbeat")
	}
}

func TestShouldEmitReconcileCycle_ActivityAlwaysEmits(t *testing.T) {
	stats := &reconcileCycleStats{scheduledAdd: 1}
	if !shouldEmitReconcileCycle(stats, 1, 60) {
		t.Fatal("scheduling activity should emit on any tick")
	}
}

func TestReconcileCycle_EmitsSummary(t *testing.T) {
	events := captureEvents(t)
	pm := &ProcessManager{
		engineName: "docker",
		ctx:        context.Background(),
	}
	start := time.Now().Add(-5 * time.Millisecond)
	stats := &reconcileCycleStats{
		scheduledAdd:    1,
		scheduledUpdate: 2,
		scheduledRemove: 0,
	}
	pm.emitReconcileCycle(start, stats, 5, 3)

	var cycle *runtimeops.RuntimeEvent
	for i := range *events {
		if (*events)[i].Event == runtimeops.EventReconcileCycle {
			cycle = &(*events)[i]
		}
	}
	if cycle == nil {
		t.Fatal("expected reconcile.cycle event")
	}
	if cycle.Level != runtimeops.LevelInfo {
		t.Fatalf("level=%q", cycle.Level)
	}
	if cycle.Source != runtimeops.SourceReconcile {
		t.Fatalf("source=%q", cycle.Source)
	}
	if cycle.DurationMs <= 0 {
		t.Fatalf("durationMs=%d", cycle.DurationMs)
	}
	fields := cycle.Fields
	if fields["desiredCount"] != 5 || fields["runningCount"] != 3 {
		t.Fatalf("counts: desired=%v running=%v", fields["desiredCount"], fields["runningCount"])
	}
	if fields["scheduledAdd"] != 1 || fields["scheduledUpdate"] != 2 || fields["scheduledRemove"] != 0 {
		t.Fatalf("scheduled: add=%v update=%v remove=%v", fields["scheduledAdd"], fields["scheduledUpdate"], fields["scheduledRemove"])
	}
}

func TestHandleLatestMicroservices_ScheduleAdd_EmitsDecision(t *testing.T) {
	events := captureEvents(t)
	t.Cleanup(func() { statusreporter.GetInstance().ResetProcessManagerStatus() })

	ms := models.NewMicroservice("ms-add", "nginx:latest")
	eng := &lifecycleTestEngine{}
	pm := &ProcessManager{
		engineName:          "docker",
		ctx:                 context.Background(),
		logger:              logging.NewModuleLogger(ProcessManagerModuleName),
		microserviceManager: &invariantMicroserviceManager{microservice: ms},
		containerManager:    newLifecycleCM(eng, &models.Registry{}),
		engine:              eng,
		taskQueue:           NewTaskQueue(10),
	}
	stats := &reconcileCycleStats{}
	pm.handleLatestMicroservices(stats)

	var decision *runtimeops.RuntimeEvent
	for i := range *events {
		e := &(*events)[i]
		if e.Event != runtimeops.EventReconcileDecision {
			continue
		}
		if e.Fields["decision"] == "ADD" {
			decision = e
		}
	}
	if decision == nil {
		t.Fatalf("expected reconcile.decision ADD, events=%v", eventNames(*events))
	}
	if decision.MsUUID != "ms-add" {
		t.Fatalf("msUUID=%q", decision.MsUUID)
	}
	if decision.Source != runtimeops.SourceReconcile {
		t.Fatalf("source=%q", decision.Source)
	}
	if decision.Fields["reason"] != "missing_container" {
		t.Fatalf("reason=%v", decision.Fields["reason"])
	}
	if stats.scheduledAdd != 1 {
		t.Fatalf("scheduledAdd=%d", stats.scheduledAdd)
	}
}
