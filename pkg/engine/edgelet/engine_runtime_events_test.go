//go:build linux

package iofog

import (
	"testing"

	eventtypes "github.com/containerd/containerd/api/events"
	"github.com/datasance/edgelet/internal/runtimeops"
)

func TestTaskExitContainerID_PrefersID(t *testing.T) {
	id := taskExitContainerID(&eventtypes.TaskExit{
		ID:          "container-abc",
		ContainerID: "other",
	})
	if id != "container-abc" {
		t.Fatalf("id=%q", id)
	}
}

func TestEmitContainerRuntimeWatchEvent_Fields(t *testing.T) {
	got := captureRuntimeEvents(t)
	e := &Engine{store: newStateStore()}
	e.store.set("cid-1", &containerState{sandboxID: "sandbox-1"})

	e.emitContainerRuntimeWatchEvent("cid-1", "ms-1", "exit", 137, "Error")

	if len(*got) != 1 {
		t.Fatalf("events=%d", len(*got))
	}
	ev := (*got)[0]
	if ev.Event != runtimeops.EventContainerRuntimeEvent {
		t.Fatalf("event=%s", ev.Event)
	}
	if ev.Level != runtimeops.LevelInfo {
		t.Fatalf("level=%s", ev.Level)
	}
	if ev.Source != runtimeops.SourceRuntimeWatch {
		t.Fatalf("source=%s", ev.Source)
	}
	if ev.Engine != edgeletEngineName {
		t.Fatalf("engine=%s", ev.Engine)
	}
	if ev.MsUUID != "ms-1" || ev.ContainerID != "cid-1" || ev.SandboxID != "sandbox-1" {
		t.Fatalf("msUUID=%q containerId=%q sandboxId=%q", ev.MsUUID, ev.ContainerID, ev.SandboxID)
	}
	if ev.Fields["runtimeStatus"] != "exit" {
		t.Fatalf("runtimeStatus=%v", ev.Fields["runtimeStatus"])
	}
	if ev.Fields["reason"] != "Error" {
		t.Fatalf("reason=%v", ev.Fields["reason"])
	}
	if ev.Fields["exitCode"] != int32(137) {
		t.Fatalf("exitCode=%v", ev.Fields["exitCode"])
	}
}

func TestEmitContainerRuntimeWatchEvent_OOM(t *testing.T) {
	got := captureRuntimeEvents(t)
	e := &Engine{store: newStateStore()}
	e.emitContainerRuntimeWatchEvent("cid-2", "ms-2", "oom", 0, "OOMKilled")

	ev := (*got)[0]
	if ev.Fields["runtimeStatus"] != "oom" {
		t.Fatalf("runtimeStatus=%v", ev.Fields["runtimeStatus"])
	}
	if _, has := ev.Fields["exitCode"]; has {
		t.Fatal("expected no exitCode for zero exit")
	}
}
