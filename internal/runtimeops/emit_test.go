package runtimeops

import (
	"context"
	"strings"
	"testing"
)

func TestEmit_InfoFieldsAppearAsTopLevelJSON(t *testing.T) {
	ctx := WithOperation(context.Background(), "op-abc", "edgelet", "ms-123")
	merged := mergeEvent(ctx, RuntimeEvent{
		Event:   EventContainerStarted,
		Level:   LevelInfo,
		Module:  "Container Manager",
		Message: "container started",
		Result:  ResultOK,
	})
	fields := merged.toFieldsMap()

	if fields["event"] != EventContainerStarted {
		t.Fatalf("event=%v want %s", fields["event"], EventContainerStarted)
	}
	if fields["operationId"] != "op-abc" {
		t.Fatalf("operationId=%v", fields["operationId"])
	}
	if fields["msUUID"] != "ms-123" {
		t.Fatalf("msUUID=%v", fields["msUUID"])
	}
	if fields["engine"] != "edgelet" {
		t.Fatalf("engine=%v", fields["engine"])
	}
}

func TestWithOperation_PropagatesToEmit(t *testing.T) {
	var got RuntimeEvent
	SetTestSink(func(e RuntimeEvent) { got = e })
	t.Cleanup(func() { SetTestSink(nil) })

	ctx := WithOperation(context.Background(), "op-1", "docker", "ms-9")
	Emit(ctx, RuntimeEvent{
		Event: EventTaskStarted,
		Level: LevelInfo,
	})

	if got.OperationID != "op-1" {
		t.Fatalf("OperationID=%q", got.OperationID)
	}
	if got.Engine != "docker" {
		t.Fatalf("Engine=%q", got.Engine)
	}
	if got.MsUUID != "ms-9" {
		t.Fatalf("MsUUID=%q", got.MsUUID)
	}
}

func TestEmit_TruncatesLongError(t *testing.T) {
	long := strings.Repeat("x", maxErrorLen+100)
	merged := mergeEvent(context.Background(), RuntimeEvent{Error: long})
	if len(merged.Error) != maxErrorLen {
		t.Fatalf("error len=%d want %d", len(merged.Error), maxErrorLen)
	}
}

func TestEmit_OmitsEmptyFields(t *testing.T) {
	m := RuntimeEvent{
		Event:       EventContainerPulling,
		OperationID: "op",
		MsUUID:      "ms",
	}.toFieldsMap()

	if _, ok := m["containerId"]; ok {
		t.Fatal("expected empty containerId omitted")
	}
	if _, ok := m["sandboxId"]; ok {
		t.Fatal("expected empty sandboxId omitted")
	}
	if _, ok := m["durationMs"]; ok {
		t.Fatal("expected zero durationMs omitted")
	}
	if _, ok := m["reasonCode"]; ok {
		t.Fatal("expected empty reasonCode omitted")
	}
}

func TestMergeEvent_EventOverridesContext(t *testing.T) {
	ctx := WithOperation(context.Background(), "ctx-op", "edgelet", "ctx-ms")
	merged := mergeEvent(ctx, RuntimeEvent{
		OperationID: "event-op",
		MsUUID:      "event-ms",
	})
	if merged.OperationID != "event-op" {
		t.Fatalf("OperationID=%q", merged.OperationID)
	}
	if merged.MsUUID != "event-ms" {
		t.Fatalf("MsUUID=%q", merged.MsUUID)
	}
	if merged.Engine != "edgelet" {
		t.Fatalf("Engine=%q", merged.Engine)
	}
}
