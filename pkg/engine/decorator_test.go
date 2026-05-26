package engine

import (
	"errors"
	"testing"

	"github.com/datasance/edgelet/internal/runtimeops"
)

type stubLoggingEngine struct {
	noopEngine
	startErr error
}

func (s *stubLoggingEngine) StartContainer(containerID string) error {
	if s.startErr != nil {
		return s.startErr
	}
	return nil
}

func (s *stubLoggingEngine) GetContainerSandboxID(string) (string, error) {
	return "sandbox-1", nil
}

func TestLoggingEngine_StartContainer_EmitsDebug(t *testing.T) {
	var events []runtimeops.RuntimeEvent
	runtimeops.SetTestSink(func(e runtimeops.RuntimeEvent) {
		events = append(events, e)
	})
	t.Cleanup(func() { runtimeops.SetTestSink(nil) })

	inner := &stubLoggingEngine{}
	eng := NewLoggingEngine(inner, "docker")
	if err := eng.StartContainer("cid-abc"); err != nil {
		t.Fatalf("StartContainer: %v", err)
	}

	var got *runtimeops.RuntimeEvent
	for i := range events {
		if events[i].Event == runtimeops.EventEngineContainerStart {
			got = &events[i]
		}
	}
	if got == nil {
		t.Fatalf("expected %s event, got %v", runtimeops.EventEngineContainerStart, eventNames(events))
	}
	if got.Level != runtimeops.LevelDebug {
		t.Fatalf("level=%q", got.Level)
	}
	if got.Engine != "docker" {
		t.Fatalf("engine=%q", got.Engine)
	}
	if got.ContainerID != "cid-abc" {
		t.Fatalf("containerId=%q", got.ContainerID)
	}
	if got.SandboxID != "sandbox-1" {
		t.Fatalf("sandboxId=%q", got.SandboxID)
	}
	if got.DurationMs < 0 {
		t.Fatalf("durationMs=%d", got.DurationMs)
	}
	if got.Result != runtimeops.ResultOK {
		t.Fatalf("result=%q", got.Result)
	}
}

func TestLoggingEngine_StartContainer_ErrorEmitsWarn(t *testing.T) {
	var events []runtimeops.RuntimeEvent
	runtimeops.SetTestSink(func(e runtimeops.RuntimeEvent) {
		events = append(events, e)
	})
	t.Cleanup(func() { runtimeops.SetTestSink(nil) })

	inner := &stubLoggingEngine{startErr: errors.New("start failed")}
	eng := NewLoggingEngine(inner, "docker")
	err := eng.StartContainer("cid-err")
	if err == nil {
		t.Fatal("expected error")
	}

	var got *runtimeops.RuntimeEvent
	for i := range events {
		if events[i].Event == runtimeops.EventEngineContainerStart {
			got = &events[i]
		}
	}
	if got == nil {
		t.Fatal("expected engine.container.start event on failure")
	}
	if got.Level != runtimeops.LevelWarn {
		t.Fatalf("level=%q", got.Level)
	}
	if got.ReasonCode != runtimeops.ReasonStartFailed {
		t.Fatalf("reasonCode=%q", got.ReasonCode)
	}
	if got.Result != runtimeops.ResultFailed {
		t.Fatalf("result=%q", got.Result)
	}
	if got.Error == "" {
		t.Fatal("expected error field")
	}
}

func eventNames(events []runtimeops.RuntimeEvent) []string {
	out := make([]string, len(events))
	for i := range events {
		out[i] = events[i].Event
	}
	return out
}

func TestLoggingEngine_PodmanEngineName(t *testing.T) {
	var events []runtimeops.RuntimeEvent
	runtimeops.SetTestSink(func(e runtimeops.RuntimeEvent) {
		events = append(events, e)
	})
	t.Cleanup(func() { runtimeops.SetTestSink(nil) })

	inner := &stubLoggingEngine{}
	eng := NewLoggingEngine(inner, "podman")
	_ = eng.StartContainer("cid-1")

	for i := range events {
		if events[i].Event == runtimeops.EventEngineContainerStart && events[i].Engine != "podman" {
			t.Fatalf("engine=%q want podman", events[i].Engine)
		}
	}
}
