package processmanager

import (
	"testing"

	"github.com/datasance/edgelet/internal/models"
	"github.com/datasance/edgelet/internal/utils/logging"
	"github.com/datasance/edgelet/pkg/engine"
)

type invariantMicroserviceManager struct {
	microservice *models.Microservice
}

func (m *invariantMicroserviceManager) GetLatestMicroservices() []*models.Microservice {
	if m.microservice == nil {
		return nil
	}
	return []*models.Microservice{m.microservice}
}

func (m *invariantMicroserviceManager) GetCurrentMicroservices() []*models.Microservice { return nil }

func (m *invariantMicroserviceManager) FindLatestMicroserviceByUUID(uuid string) *models.Microservice {
	if m.microservice != nil && m.microservice.MicroserviceUUID == uuid {
		return m.microservice
	}
	return nil
}

func (m *invariantMicroserviceManager) GetRegistry(_ int) *models.Registry { return nil }

func (m *invariantMicroserviceManager) SetCurrentMicroservices(_ []*models.Microservice) {}

// These tests lock in guard-rail behavior for the process-manager reconciliation path.
func TestShouldContainerBeUpdated_GuardRails(t *testing.T) {
	pm := &ProcessManager{logger: logging.NewModuleLogger(ProcessManagerModuleName)}
	container := &engine.Container{ID: "c1"}
	ms := models.NewMicroservice("ms-1", "nginx:latest")

	// Invariant: never trigger an update while already in-flight.
	ms.SetIsUpdating(true)
	status := models.NewMicroserviceStatusWithState(models.MicroserviceStateRunning)
	if pm.shouldContainerBeUpdated(ms, container, status) {
		t.Fatal("expected false when microservice is already updating")
	}

	// Invariant: explicit rebuild always requests an update and bypasses state checks.
	ms.SetIsUpdating(false)
	ms.Rebuild = true
	if !pm.shouldContainerBeUpdated(ms, container, status) {
		t.Fatal("expected true when rebuild is explicitly requested")
	}
	ms.Rebuild = false

	// Invariant: these transitional states are reconciliation no-op states.
	blockedStates := []models.MicroserviceState{
		models.MicroserviceStateQueued,
		models.MicroserviceStateUpdating,
		models.MicroserviceStateStuckInRestart,
	}
	for _, st := range blockedStates {
		st := st
		t.Run(string(st), func(t *testing.T) {
			s := models.NewMicroserviceStatusWithState(st)
			if pm.shouldContainerBeUpdated(ms, container, s) {
				t.Fatalf("expected false for guarded transitional state %s", st)
			}
		})
	}
}

func TestExecuteTask_CreateExecIsFacadeOnly(t *testing.T) {
	ms := models.NewMicroservice("ms-1", "nginx:latest")
	pm := &ProcessManager{
		microserviceManager: &invariantMicroserviceManager{microservice: ms},
		logger:              logging.NewModuleLogger(ProcessManagerModuleName),
	}

	// Invariant: TaskActionCreateExec does not execute runtime changes here.
	task := NewContainerTask(TaskActionCreateExec, ms.MicroserviceUUID)
	if err := pm.executeTask(task); err != nil {
		t.Fatalf("expected nil error for create-exec task façade path, got %v", err)
	}
}
