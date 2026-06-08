package docker

import (
	"errors"
	"testing"

	"github.com/docker/docker/api/types/events"
	"github.com/eclipse-iofog/edgelet/internal/runtimeops"
	"github.com/eclipse-iofog/edgelet/internal/utils/logging"
	"github.com/eclipse-iofog/edgelet/internal/workloadmeta"
)

func captureRuntimeEvents(t *testing.T) *[]runtimeops.RuntimeEvent {
	t.Helper()
	out := &[]runtimeops.RuntimeEvent{}
	runtimeops.SetTestSink(func(e runtimeops.RuntimeEvent) {
		*out = append(*out, e)
	})
	t.Cleanup(func() { runtimeops.SetTestSink(nil) })
	return out
}

func managedEventAttributes(msUUID string) map[string]string {
	return map[string]string{
		"label." + workloadmeta.LabelAppManagedBy:    workloadmeta.ManagedByValue,
		"label." + workloadmeta.LabelMicroserviceUID: msUUID,
	}
}

func TestDockerEventHandler_EmitsForLabeledContainer(t *testing.T) {
	got := captureRuntimeEvents(t)
	c := &Client{logger: nil}

	c.handleDockerEvent(events.Message{
		Type:   "container",
		Status: "die",
		Action: events.ActionDie,
		ID:     "cid-managed",
		Actor: events.Actor{
			ID:         "cid-managed",
			Attributes: managedEventAttributes("ms-managed"),
		},
	})

	var runtime *runtimeops.RuntimeEvent
	for i := range *got {
		if (*got)[i].Event == runtimeops.EventContainerRuntimeEvent && (*got)[i].Level == runtimeops.LevelInfo {
			runtime = &(*got)[i]
		}
	}
	if runtime == nil {
		t.Fatalf("expected Info %s, got %d events", runtimeops.EventContainerRuntimeEvent, len(*got))
	}
	if runtime.Source != runtimeops.SourceRuntimeWatch {
		t.Fatalf("source=%q", runtime.Source)
	}
	if runtime.Engine != dockerEngineName {
		t.Fatalf("engine=%q", runtime.Engine)
	}
	if runtime.MsUUID != "ms-managed" {
		t.Fatalf("msUUID=%q", runtime.MsUUID)
	}
	if runtime.ContainerID != "cid-managed" {
		t.Fatalf("containerId=%q", runtime.ContainerID)
	}
	if runtime.Fields["runtimeStatus"] != "die" {
		t.Fatalf("runtimeStatus=%v", runtime.Fields["runtimeStatus"])
	}
}

func TestDockerEventHandler_IgnoresUnlabeled(t *testing.T) {
	got := captureRuntimeEvents(t)
	c := &Client{logger: logging.NewModuleLogger(ModuleName)}

	c.handleDockerEvent(events.Message{
		Type:   "container",
		Status: "die",
		ID:     "cid-other",
		Actor: events.Actor{
			Attributes: map[string]string{
				"label." + workloadmeta.LabelMicroserviceUID: "ms-other",
			},
		},
	})

	for i := range *got {
		ev := &(*got)[i]
		if ev.Event == runtimeops.EventContainerRuntimeEvent && ev.Level == runtimeops.LevelInfo {
			t.Fatalf("unexpected Info runtime event for unlabeled container: %+v", ev)
		}
	}
}

func TestDockerEventHandler_StreamErrorEmitsWarn(t *testing.T) {
	got := captureRuntimeEvents(t)

	emitDockerEventsStreamError(errors.New("connection reset"))

	var runtime *runtimeops.RuntimeEvent
	for i := range *got {
		if (*got)[i].Event == runtimeops.EventContainerRuntimeEvent {
			runtime = &(*got)[i]
		}
	}
	if runtime == nil {
		t.Fatal("expected container.runtime.event on stream error")
	}
	if runtime.Level != runtimeops.LevelWarn {
		t.Fatalf("level=%q", runtime.Level)
	}
	if runtime.Fields["runtimeStatus"] != "stream_error" {
		t.Fatalf("runtimeStatus=%v", runtime.Fields["runtimeStatus"])
	}
	if runtime.Error == "" {
		t.Fatal("expected error field")
	}
}

func TestLabelsFromDockerEventAttributes(t *testing.T) {
	labels := labelsFromDockerEventAttributes(managedEventAttributes("ms-1"))
	if !workloadmeta.IsManagedByIofog(labels) {
		t.Fatalf("labels=%v", labels)
	}
}
