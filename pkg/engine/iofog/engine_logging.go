//go:build linux

package iofog

import (
	"context"
	"time"

	"github.com/datasance/edgelet/internal/runtimeops"
)

const (
	edgeletEngineName    = "edgelet"
	iofogRuntimeModule = "IofogEngine"
)

func (e *Engine) runtimeLogCtx() context.Context {
	return context.Background()
}

func (e *Engine) sandboxIDFor(containerID string) string {
	if st, ok := e.store.get(containerID); ok {
		return st.sandboxID
	}
	return ""
}

func (e *Engine) emitRuntime(ev runtimeops.RuntimeEvent) {
	if ev.Engine == "" {
		ev.Engine = edgeletEngineName
	}
	if ev.Module == "" {
		ev.Module = iofogRuntimeModule
	}
	runtimeops.Emit(e.runtimeLogCtx(), ev)
}

func (e *Engine) emitEngineSuccess(event, containerID, sandboxID, image, message string, start time.Time, fields map[string]any) {
	e.emitRuntime(runtimeops.RuntimeEvent{
		Event:       event,
		Level:       runtimeops.LevelDebug,
		ContainerID: containerID,
		SandboxID:   sandboxID,
		Image:       image,
		Message:     message,
		Result:      runtimeops.ResultOK,
		DurationMs:  time.Since(start).Milliseconds(),
		Fields:      fields,
	})
}

func (e *Engine) emitEngineInfo(event, containerID, sandboxID, image, message string, start time.Time, fields map[string]any) {
	e.emitRuntime(runtimeops.RuntimeEvent{
		Event:       event,
		Level:       runtimeops.LevelInfo,
		ContainerID: containerID,
		SandboxID:   sandboxID,
		Image:       image,
		Message:     message,
		Result:      runtimeops.ResultOK,
		DurationMs:  time.Since(start).Milliseconds(),
		Fields:      fields,
	})
}

func (e *Engine) emitEngineWarn(event, containerID, sandboxID, image, reasonCode, message string, start time.Time, err error, fields map[string]any) {
	ev := runtimeops.RuntimeEvent{
		Event:       event,
		Level:       runtimeops.LevelWarn,
		ContainerID: containerID,
		SandboxID:   sandboxID,
		Image:       image,
		ReasonCode:  reasonCode,
		Message:     message,
		Result:      runtimeops.ResultFailed,
		DurationMs:  time.Since(start).Milliseconds(),
		Fields:      fields,
	}
	if err != nil {
		ev.Error = err.Error()
	}
	e.emitRuntime(ev)
}

func (e *Engine) emitEnginePruneSummary(operation string, start time.Time, fields map[string]any) {
	if fields == nil {
		fields = map[string]any{}
	}
	fields["operation"] = operation
	e.emitRuntime(runtimeops.RuntimeEvent{
		Event:      runtimeops.EventEnginePrune,
		Level:      runtimeops.LevelInfo,
		Message:    operation + " completed",
		Result:     runtimeops.ResultOK,
		DurationMs: time.Since(start).Milliseconds(),
		Fields:     fields,
	})
}

func (e *Engine) emitCRITeardownStep(step, containerID, sandboxID string, start time.Time, err error, tolerated bool) {
	fields := map[string]any{"step": step}
	if tolerated {
		fields["tolerated"] = true
	}
	if err != nil {
		e.emitEngineWarn(runtimeops.EventEngineContainerRemove, containerID, sandboxID, "", runtimeops.ReasonRemoveFailed, "CRI teardown step failed", start, err, fields)
		return
	}
	e.emitEngineSuccess(runtimeops.EventEngineContainerRemove, containerID, sandboxID, "", "CRI teardown step complete", start, fields)
}

func (e *Engine) emitCRISubstep(event, step, containerID, sandboxID, image, reasonCode string, start time.Time, err error) {
	fields := map[string]any{"step": step}
	if err != nil {
		e.emitEngineWarn(event, containerID, sandboxID, image, reasonCode, "CRI substep failed", start, err, fields)
		return
	}
	e.emitEngineSuccess(event, containerID, sandboxID, image, "CRI substep complete", start, fields)
}
