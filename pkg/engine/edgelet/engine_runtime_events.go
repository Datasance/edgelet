//go:build linux

package edgelet

import (
	"context"
	"errors"
	"strings"

	eventtypes "github.com/containerd/containerd/api/events"
	"github.com/containerd/containerd/v2/core/events"
	"github.com/containerd/typeurl/v2"
	"github.com/eclipse-iofog/edgelet/internal/runtimeops"
	"github.com/eclipse-iofog/edgelet/internal/workloadmeta"
)

func (e *Engine) startContainerdRuntimeEventMonitor() {
	if e.client == nil {
		return
	}
	ctx, cancel := context.WithCancel(e.ctx())
	e.runtimeEventsCancel = cancel
	go e.runContainerdRuntimeEventMonitor(ctx)
}

func (e *Engine) runContainerdRuntimeEventMonitor(ctx context.Context) {
	ch, errCh := e.client.Subscribe(ctx, `topic=="/tasks/exit"`, `topic=="/tasks/oom"`)
	for {
		select {
		case <-ctx.Done():
			return
		case env, ok := <-ch:
			if !ok {
				return
			}
			if env != nil {
				e.handleContainerdEventEnvelope(ctx, env)
			}
		case err, ok := <-errCh:
			if !ok {
				return
			}
			if err == nil {
				return
			}
			if errors.Is(err, context.Canceled) {
				return
			}
			e.emitRuntime(runtimeops.RuntimeEvent{
				Event:   runtimeops.EventContainerRuntimeEvent,
				Level:   runtimeops.LevelWarn,
				Message: "containerd events stream error",
				Result:  runtimeops.ResultFailed,
				Source:  runtimeops.SourceRuntimeWatch,
				Error:   err.Error(),
				Fields: map[string]any{
					"runtimeStatus": "stream_error",
				},
			})
		}
	}
}

func (e *Engine) handleContainerdEventEnvelope(ctx context.Context, env *events.Envelope) {
	ev, err := typeurl.UnmarshalAny(env.Event)
	if err != nil {
		return
	}
	switch evt := ev.(type) {
	case *eventtypes.TaskExit:
		e.handleTaskExitRuntimeEvent(ctx, evt)
	case *eventtypes.TaskOOM:
		e.handleTaskOOMRuntimeEvent(ctx, evt)
	}
}

func (e *Engine) handleTaskExitRuntimeEvent(ctx context.Context, ev *eventtypes.TaskExit) {
	containerID := taskExitContainerID(ev)
	if containerID == "" {
		return
	}
	msUUID, ok := e.managedMicroserviceForContainer(ctx, containerID)
	if !ok {
		return
	}
	reason, exitCode, _ := e.readCRIContainerFailure(ctx, containerID)
	if reason == "" {
		reason = "ContainerExited"
	}
	e.emitContainerRuntimeWatchEvent(containerID, msUUID, "exit", exitCode, reason)
}

func (e *Engine) handleTaskOOMRuntimeEvent(ctx context.Context, ev *eventtypes.TaskOOM) {
	containerID := strings.TrimSpace(ev.ContainerID)
	if containerID == "" {
		return
	}
	msUUID, ok := e.managedMicroserviceForContainer(ctx, containerID)
	if !ok {
		return
	}
	reason, exitCode, _ := e.readCRIContainerFailure(ctx, containerID)
	if reason == "" {
		reason = "OOMKilled"
	}
	e.emitContainerRuntimeWatchEvent(containerID, msUUID, "oom", exitCode, reason)
}

func (e *Engine) managedMicroserviceForContainer(ctx context.Context, containerID string) (msUUID string, ok bool) {
	c, err := e.client.LoadContainer(ctx, containerID)
	if err != nil {
		return "", false
	}
	if isSandboxContainer(ctx, c) {
		return "", false
	}
	info, err := c.Info(ctx)
	if err != nil || info.Labels == nil {
		return "", false
	}
	if !workloadmeta.IsManagedByIofog(info.Labels) {
		return "", false
	}
	msUUID = workloadmeta.MicroserviceUIDFromLabels(info.Labels)
	if msUUID == "" {
		return "", false
	}
	return msUUID, true
}

func (e *Engine) emitContainerRuntimeWatchEvent(containerID, msUUID, runtimeStatus string, exitCode int32, reason string) {
	fields := map[string]any{
		"runtimeStatus": runtimeStatus,
		"reason":        reason,
	}
	if exitCode != 0 {
		fields["exitCode"] = exitCode
	}
	e.emitRuntime(runtimeops.RuntimeEvent{
		Event:       runtimeops.EventContainerRuntimeEvent,
		Level:       runtimeops.LevelInfo,
		MsUUID:      msUUID,
		ContainerID: containerID,
		SandboxID:   e.sandboxIDFor(containerID),
		Source:      runtimeops.SourceRuntimeWatch,
		Message:     "container runtime event",
		Fields:      fields,
	})
}

func taskExitContainerID(ev *eventtypes.TaskExit) string {
	// Match CRI: workload task exit uses ID as the container ID; exec exits use a different ID.
	if id := strings.TrimSpace(ev.ID); id != "" {
		return id
	}
	return strings.TrimSpace(ev.ContainerID)
}
