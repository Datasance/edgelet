package docker

import (
	"context"
	"errors"
	"strings"

	"github.com/docker/docker/api/types/events"
	"github.com/eclipse-iofog/agent/internal/models"
	"github.com/eclipse-iofog/agent/internal/runtimeops"
	"github.com/eclipse-iofog/agent/internal/workloadmeta"
)

const dockerEngineName = "docker"

// addDockerEventHandler starts the Docker events handler.
func (c *Client) addDockerEventHandler() {
	c.logger.Debug("Starting docker events handler")

	cli := c.GetClient()
	if cli == nil {
		c.logger.Error("Docker client not initialized")
		return
	}

	ctx := c.GetContext()
	eventCh, errCh := cli.Events(ctx, events.ListOptions{})

	go func() {
		for {
			select {
			case event, ok := <-eventCh:
				if !ok {
					return
				}
				c.handleDockerEvent(event)
			case err, ok := <-errCh:
				if !ok {
					return
				}
				if err != nil {
					c.handleDockerEventsStreamError(err)
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	c.logger.Debug("Docker events handler started")
}

func (c *Client) handleDockerEvent(event events.Message) {
	if event.Type != "container" && event.Type != "image" {
		return
	}

	labels := labelsFromDockerEventAttributes(event.Actor.Attributes)
	if event.Type == "container" && workloadmeta.IsManagedByIofog(labels) {
		c.emitManagedContainerRuntimeEvent(event, labels)
		return
	}

	state := models.MicroserviceStateFromText(event.Status)
	c.logger.Debugf("Docker event: Type=%s, Status=%s, ID=%s, State=%s", event.Type, event.Status, containerIDFromEvent(event), state)
}

func (c *Client) emitManagedContainerRuntimeEvent(event events.Message, labels map[string]string) {
	msUUID := workloadmeta.MicroserviceUIDFromLabels(labels)
	containerID := containerIDFromEvent(event)
	runtimeStatus := runtimeStatusFromDockerEvent(event)

	runtimeops.Emit(context.Background(), runtimeops.RuntimeEvent{
		Event:       runtimeops.EventContainerRuntimeEvent,
		Level:       runtimeops.LevelInfo,
		Module:      ModuleName,
		Engine:      dockerEngineName,
		MsUUID:      msUUID,
		ContainerID: containerID,
		Source:      runtimeops.SourceRuntimeWatch,
		Message:     "container runtime event",
		Fields: map[string]any{
			"runtimeStatus": runtimeStatus,
		},
	})
}

func emitDockerEventsStreamError(err error) {
	runtimeops.Emit(context.Background(), runtimeops.RuntimeEvent{
		Event:   runtimeops.EventContainerRuntimeEvent,
		Level:   runtimeops.LevelWarn,
		Module:  ModuleName,
		Engine:  dockerEngineName,
		Source:  runtimeops.SourceRuntimeWatch,
		Message: "docker events stream error",
		Result:  runtimeops.ResultFailed,
		Error:   err.Error(),
		Fields: map[string]any{
			"runtimeStatus": "stream_error",
		},
	})
}

func (c *Client) handleDockerEventsStreamError(err error) {
	if errors.Is(err, context.Canceled) {
		return
	}
	emitDockerEventsStreamError(err)
	c.logger.Warnf("Docker events error: %v", err)
	if reinitErr := c.ReInit(); reinitErr != nil {
		c.logger.Errorf("Failed to reinitialize Docker client: %v", reinitErr)
	}
}

func labelsFromDockerEventAttributes(attrs map[string]string) map[string]string {
	if len(attrs) == 0 {
		return nil
	}
	const prefix = "label."
	labels := make(map[string]string)
	for k, v := range attrs {
		if strings.HasPrefix(k, prefix) {
			labels[k[len(prefix):]] = v
		}
	}
	if len(labels) == 0 {
		return nil
	}
	return labels
}

func containerIDFromEvent(event events.Message) string {
	if id := strings.TrimSpace(event.ID); id != "" {
		return id
	}
	return strings.TrimSpace(event.Actor.ID)
}

func runtimeStatusFromDockerEvent(event events.Message) string {
	if action := strings.TrimSpace(string(event.Action)); action != "" {
		return strings.ToLower(action)
	}
	return strings.ToLower(strings.TrimSpace(event.Status))
}
