package processmanager

import (
	"context"
	"time"

	"github.com/eclipse-iofog/agent/internal/runtimeops"
)

// emitFromCM logs a structured runtime event, filling module and engine when unset.
// operationId, engine, and msUUID are merged from ctx by runtimeops.Emit.
func (cm *ContainerManager) emitFromCM(ctx context.Context, e runtimeops.RuntimeEvent) {
	if e.Module == "" {
		e.Module = ContainerManagerModuleName
	}
	if e.Engine == "" {
		e.Engine = cm.engineName
	}
	if e.Level == "" {
		e.Level = runtimeops.LevelInfo
	}
	runtimeops.Emit(ctx, e)
}

// removeRuntimeContainer stops and removes a workload, emitting lifecycle audit events.
func (cm *ContainerManager) removeRuntimeContainer(
	ctx context.Context,
	msUUID, containerID, image, source string,
	withCleanup, removeImage bool,
) error {
	cm.emitFromCM(ctx, runtimeops.RuntimeEvent{
		Event:       runtimeops.EventContainerStopping,
		MsUUID:      msUUID,
		ContainerID: containerID,
		Image:       image,
		Source:      source,
		Message:     "stopping container",
	})

	stopStart := time.Now()
	stopErr := cm.engine.StopContainer(containerID)
	stopEvent := runtimeops.RuntimeEvent{
		Event:       runtimeops.EventContainerStopped,
		MsUUID:      msUUID,
		ContainerID: containerID,
		Image:       image,
		Source:      source,
		DurationMs:  time.Since(stopStart).Milliseconds(),
		Message:     "container stopped",
	}
	if stopErr != nil {
		stopEvent.Level = runtimeops.LevelWarn
		stopEvent.Result = runtimeops.ResultFailed
		stopEvent.ReasonCode = runtimeops.ReasonStopFailed
		stopEvent.Error = stopErr.Error()
		stopEvent.Message = "container stop failed"
	} else {
		stopEvent.Result = runtimeops.ResultOK
	}
	cm.emitFromCM(ctx, stopEvent)

	cm.emitFromCM(ctx, runtimeops.RuntimeEvent{
		Event:       runtimeops.EventContainerRemoving,
		MsUUID:      msUUID,
		ContainerID: containerID,
		Image:       image,
		Source:      source,
		Message:     "removing container",
	})

	removeStart := time.Now()
	if err := cm.engine.RemoveContainer(containerID, withCleanup); err != nil {
		cm.emitFromCM(ctx, runtimeops.RuntimeEvent{
			Event:       runtimeops.EventContainerRemoved,
			Level:       runtimeops.LevelError,
			MsUUID:      msUUID,
			ContainerID: containerID,
			Image:       image,
			Source:      source,
			Result:      runtimeops.ResultFailed,
			ReasonCode:  runtimeops.ReasonRemoveFailed,
			DurationMs:  time.Since(removeStart).Milliseconds(),
			Error:       err.Error(),
			Message:     "container remove failed",
		})
		return err
	}

	cm.emitFromCM(ctx, runtimeops.RuntimeEvent{
		Event:       runtimeops.EventContainerRemoved,
		MsUUID:      msUUID,
		ContainerID: containerID,
		Image:       image,
		Source:      source,
		Result:      runtimeops.ResultOK,
		DurationMs:  time.Since(removeStart).Milliseconds(),
		Message:     "container removed",
	})

	if removeImage && image != "" {
		if err := cm.engine.RemoveImage(image); err != nil {
			cm.logger.Warnf("Image %s cannot be removed (may be in use): %v", image, err)
		}
	}
	return nil
}
