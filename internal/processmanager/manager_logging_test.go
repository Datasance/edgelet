package processmanager

import (
	"context"
	"errors"
	"testing"

	"github.com/datasance/edgelet/internal/models"
	"github.com/datasance/edgelet/internal/runtimeops"
	"github.com/datasance/edgelet/internal/utils/logging"
)

func TestExecuteTask_EmitsTaskCompletedOnSuccess(t *testing.T) {
	var events []runtimeops.RuntimeEvent
	runtimeops.SetTestSink(func(e runtimeops.RuntimeEvent) {
		events = append(events, e)
	})
	t.Cleanup(func() { runtimeops.SetTestSink(nil) })

	ms := models.NewMicroservice("ms-1", "nginx:latest")
	pm := &ProcessManager{
		engineName:          "docker",
		microserviceManager: &invariantMicroserviceManager{microservice: ms},
		ctx:                 context.Background(),
		logger:              logging.NewModuleLogger(ProcessManagerModuleName),
	}

	task := NewContainerTask(TaskActionCreateExec, ms.MicroserviceUUID)
	if err := pm.executeTask(task); err != nil {
		t.Fatalf("executeTask: %v", err)
	}

	var started, completed *runtimeops.RuntimeEvent
	for i := range events {
		switch events[i].Event {
		case runtimeops.EventTaskStarted:
			started = &events[i]
		case runtimeops.EventTaskCompleted:
			completed = &events[i]
		}
	}
	if started == nil || completed == nil {
		t.Fatalf("expected task.started and task.completed, got %d events", len(events))
	}
	if started.OperationID != completed.OperationID || started.OperationID == "" {
		t.Fatalf("operationId mismatch started=%q completed=%q", started.OperationID, completed.OperationID)
	}
	if started.Engine != "docker" || completed.Engine != "docker" {
		t.Fatalf("engine=%q", started.Engine)
	}
	if task.OperationID != started.OperationID {
		t.Fatalf("task.OperationID=%q started=%q", task.OperationID, started.OperationID)
	}
}

func TestEmitTaskFailed_AfterMaxRetries(t *testing.T) {
	var events []runtimeops.RuntimeEvent
	runtimeops.SetTestSink(func(e runtimeops.RuntimeEvent) {
		events = append(events, e)
	})
	t.Cleanup(func() { runtimeops.SetTestSink(nil) })

	pm := &ProcessManager{
		engineName: "edgelet",
		ctx:        context.Background(),
		logger:     logging.NewModuleLogger(ProcessManagerModuleName),
	}
	task := NewContainerTask(TaskActionAdd, "ms-1")
	task.OperationID = "fixed-op-id"
	task.Retries = 5

	pm.emitTaskFailed(task, errors.New("boom"))

	var failed *runtimeops.RuntimeEvent
	for i := range events {
		if events[i].Event == runtimeops.EventTaskFailed {
			failed = &events[i]
		}
	}
	if failed == nil {
		t.Fatal("expected task.failed event")
	}
	if failed.OperationID != "fixed-op-id" {
		t.Fatalf("operationId=%q", failed.OperationID)
	}
	if failed.Engine != "edgelet" {
		t.Fatalf("engine=%q", failed.Engine)
	}
	if failed.ReasonCode != runtimeops.ReasonTaskExhaustedRetries {
		t.Fatalf("reasonCode=%q", failed.ReasonCode)
	}
}

func TestAddTask_EmitsEnqueued(t *testing.T) {
	events := captureEvents(t)
	pm := &ProcessManager{
		engineName: "docker",
		ctx:        context.Background(),
		taskQueue:  NewTaskQueue(10),
		logger:     logging.NewModuleLogger(ProcessManagerModuleName),
	}

	task := NewContainerTask(TaskActionAdd, "ms-enq")
	pm.addTask(task)

	var enqueued *runtimeops.RuntimeEvent
	for i := range *events {
		if (*events)[i].Event == runtimeops.EventTaskEnqueued {
			enqueued = &(*events)[i]
		}
	}
	if enqueued == nil {
		t.Fatal("expected task.enqueued event")
	}
	if enqueued.OperationID == "" {
		t.Fatal("expected operationId on enqueued event")
	}
	if task.OperationID != enqueued.OperationID {
		t.Fatalf("task.OperationID=%q enqueued=%q", task.OperationID, enqueued.OperationID)
	}
	if enqueued.Engine != "docker" {
		t.Fatalf("engine=%q", enqueued.Engine)
	}
	if enqueued.Fields["action"] != string(TaskActionAdd) {
		t.Fatalf("action=%v", enqueued.Fields["action"])
	}
	if depth, ok := enqueued.Fields["queueDepth"].(int); !ok || depth < 1 {
		t.Fatalf("queueDepth=%v", enqueued.Fields["queueDepth"])
	}
}

func TestExecuteTask_RetryEmitsWarn(t *testing.T) {
	events := captureEvents(t)
	pm := &ProcessManager{
		engineName: "edgelet",
		ctx:        context.Background(),
		taskQueue:  NewTaskQueue(10),
		logger:     logging.NewModuleLogger(ProcessManagerModuleName),
	}

	task := NewContainerTask(TaskActionUpdate, "ms-retry")
	task.OperationID = "op-retry"
	pm.retryTask(task)

	var retry *runtimeops.RuntimeEvent
	for i := range *events {
		if (*events)[i].Event == runtimeops.EventTaskRetry {
			retry = &(*events)[i]
		}
	}
	if retry == nil {
		t.Fatal("expected task.retry event")
	}
	if retry.OperationID != "op-retry" {
		t.Fatalf("operationId=%q", retry.OperationID)
	}
	if retry.Level != runtimeops.LevelWarn {
		t.Fatalf("level=%q", retry.Level)
	}
	if retry.Fields["attempt"] != 1 {
		t.Fatalf("attempt=%v", retry.Fields["attempt"])
	}
	if retry.Fields["maxRetries"] != maxTaskRetries {
		t.Fatalf("maxRetries=%v", retry.Fields["maxRetries"])
	}
	if task.Retries != 1 {
		t.Fatalf("task.Retries=%d", task.Retries)
	}
}

func TestTaskFailureChain_StableOperationID(t *testing.T) {
	events := captureEvents(t)
	pm := &ProcessManager{
		engineName: "docker",
		ctx:        context.Background(),
		taskQueue:  NewTaskQueue(10),
		logger:     logging.NewModuleLogger(ProcessManagerModuleName),
	}

	task := NewContainerTask(TaskActionAdd, "ms-chain")
	pm.addTask(task)
	opID := task.OperationID

	runtimeops.Emit(pm.ctx, runtimeops.RuntimeEvent{
		Event:       runtimeops.EventTaskStarted,
		Level:       runtimeops.LevelInfo,
		Module:      ProcessManagerModuleName,
		OperationID: opID,
		Engine:      pm.engineName,
		MsUUID:      task.MicroserviceUUID,
		Source:      runtimeops.SourceTask,
		Message:     "task started",
		Fields:      map[string]any{"action": string(task.Action)},
	})
	pm.retryTask(task)
	task.Retries = maxTaskRetries
	pm.emitTaskFailed(task, errors.New("chain failure"))

	var enqueued, started, retry, failed bool
	for _, e := range *events {
		if e.OperationID != opID {
			continue
		}
		switch e.Event {
		case runtimeops.EventTaskEnqueued:
			enqueued = true
		case runtimeops.EventTaskStarted:
			started = true
		case runtimeops.EventTaskRetry:
			retry = true
		case runtimeops.EventTaskFailed:
			failed = e.ReasonCode == runtimeops.ReasonTaskExhaustedRetries
		}
	}
	if !enqueued || !started || !retry || !failed {
		t.Fatalf("chain events enqueued=%v started=%v retry=%v failed=%v", enqueued, started, retry, failed)
	}
}
