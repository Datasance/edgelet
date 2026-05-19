package processmanager

import (
	"context"
	"time"

	"github.com/eclipse-iofog/agent/internal/models"
	"github.com/eclipse-iofog/agent/internal/runtimeops"
	"github.com/google/uuid"
)

// reconcileCycleStats counts reconcile scheduling actions in one monitor tick.
type reconcileCycleStats struct {
	scheduledAdd    int
	scheduledUpdate int
	scheduledRemove int
}

func (pm *ProcessManager) reconcileOperationContext(msUUID string) context.Context {
	base := pm.ctx
	if base == nil {
		base = context.Background()
	}
	return runtimeops.WithOperation(base, uuid.NewString(), pm.engineName, msUUID)
}

func (pm *ProcessManager) emitReconcileDecision(msUUID, decision, reason, message, level string, extra map[string]any) {
	if level == "" {
		level = runtimeops.LevelInfo
	}
	fields := map[string]any{
		"decision": decision,
		"reason":   reason,
	}
	for k, v := range extra {
		fields[k] = v
	}
	runtimeops.Emit(pm.ctx, runtimeops.RuntimeEvent{
		Event:   runtimeops.EventReconcileDecision,
		Level:   level,
		Module:  ProcessManagerModuleName,
		MsUUID:  msUUID,
		Engine:  pm.engineName,
		Source:  runtimeops.SourceReconcile,
		Message: message,
		Fields:  fields,
	})
}

func reconcileUpdateReason(ms *models.Microservice, status *models.MicroserviceStatus) string {
	if ms.Rebuild {
		return "rebuild"
	}
	if status.Status != models.MicroserviceStateRunning {
		return "not_running"
	}
	return "config_drift"
}

// shouldEmitReconcileCycle returns true when reconcile.cycle should be logged at Info:
// any scheduling activity in the tick, or every everyNTicks monitor iterations when idle.
func shouldEmitReconcileCycle(stats *reconcileCycleStats, tick uint64, everyNTicks int) bool {
	if stats != nil && stats.scheduledAdd+stats.scheduledUpdate+stats.scheduledRemove > 0 {
		return true
	}
	if everyNTicks < 1 {
		everyNTicks = 60
	}
	return tick > 0 && tick%uint64(everyNTicks) == 0
}

func (pm *ProcessManager) emitReconcileCycle(start time.Time, stats *reconcileCycleStats, desiredCount, runningCount int) {
	if stats == nil {
		stats = &reconcileCycleStats{}
	}
	runtimeops.Emit(pm.ctx, runtimeops.RuntimeEvent{
		Event:      runtimeops.EventReconcileCycle,
		Level:      runtimeops.LevelInfo,
		Module:     ProcessManagerModuleName,
		Engine:     pm.engineName,
		Source:     runtimeops.SourceReconcile,
		DurationMs: time.Since(start).Milliseconds(),
		Message:    "reconcile cycle complete",
		Fields: map[string]any{
			"desiredCount":    desiredCount,
			"scheduledAdd":    stats.scheduledAdd,
			"scheduledUpdate": stats.scheduledUpdate,
			"scheduledRemove": stats.scheduledRemove,
			"runningCount":    runningCount,
		},
	})
}

func (pm *ProcessManager) emitShutdownDrain(message string, level string, reasonCode string, extra map[string]any) {
	if level == "" {
		level = runtimeops.LevelInfo
	}
	runtimeops.Emit(pm.ctx, runtimeops.RuntimeEvent{
		Event:      runtimeops.EventShutdownDrain,
		Level:      level,
		Module:     ProcessManagerModuleName,
		Engine:     pm.engineName,
		Source:     runtimeops.SourceShutdown,
		ReasonCode: reasonCode,
		Message:    message,
		Fields:     extra,
	})
}
