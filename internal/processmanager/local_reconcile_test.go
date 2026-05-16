package processmanager

import (
	"errors"
	"testing"

	"github.com/eclipse-iofog/agent/internal/models"
	"github.com/eclipse-iofog/agent/internal/store"
	"github.com/eclipse-iofog/agent/internal/utils/logging"
	"github.com/eclipse-iofog/agent/pkg/engine"
)

func TestBumpLocalFailureMarksStuckAfterThreshold(t *testing.T) {
	pm := &ProcessManager{}
	pm.logger = logging.NewModuleLogger("test-process-manager")
	item := &models.LocalDeployedMicroservice{
		LocalUUID:    "local-1",
		RuntimeState: "failed",
		State:        "failed",
		FailureCount: localReconcileMaxFailures - 1,
		RestartCount: 2,
	}

	pm.bumpLocalFailure(item, nil, "failed")

	if item.FailureCount != localReconcileMaxFailures {
		t.Fatalf("expected failure_count=%d, got %d", localReconcileMaxFailures, item.FailureCount)
	}
	if item.RuntimeState != "stuck_in_restart" {
		t.Fatalf("expected runtime_state=stuck_in_restart, got %q", item.RuntimeState)
	}
	if item.State != "stuck_in_restart" {
		t.Fatalf("expected state=stuck_in_restart, got %q", item.State)
	}
	if item.RestartCount != 3 {
		t.Fatalf("expected restart_count increment, got %d", item.RestartCount)
	}
}

func TestReconcileLocalDesiredRunning_CreatedNonRestartableRecreates(t *testing.T) {
	openLocalReconcileTestDB(t)

	pm := &ProcessManager{}
	pm.logger = logging.NewModuleLogger("test-process-manager")
	item := &models.LocalDeployedMicroservice{
		LocalUUID:    "local-1",
		ManifestYAML: minimalLocalManifestYAML(),
		Generation:   1,
		FailureCount: 2,
		RuntimeState: "created",
		State:        "created",
		ContainerID:  "old-container",
		DesiredState: "running",
	}
	container := &engine.Container{ID: "old-container"}
	removeCalled := false
	launchCalled := false
	pm.startMicroserviceFn = func(_ string) error {
		return &engine.NonRestartableContainerError{
			ContainerID: "old-container",
			Reason:      "CONTAINER_EXITED",
			ExitCode:    255,
			Message:     "container is in CONTAINER_EXITED state",
		}
	}
	pm.removeContainerByIDFn = func(containerID string) error {
		if containerID != "old-container" {
			t.Fatalf("unexpected container id: %s", containerID)
		}
		removeCalled = true
		return nil
	}
	pm.launchLocalDeploymentFn = func(target *models.LocalDeployedMicroservice, _ int64) {
		launchCalled = true
		target.ContainerID = "new-container"
		target.RuntimeState = "running"
		target.State = "running"
		target.LastError = ""
		target.FailureCount = 0
	}
	pm.getContainerStatusFn = func(_, _ string) (*models.MicroserviceStatus, error) {
		return &models.MicroserviceStatus{Status: models.MicroserviceStateCreated}, nil
	}

	pm.reconcileLocalDesiredRunning(item, container, 123)

	if !removeCalled {
		t.Fatalf("expected remove to be called for non-restartable created container")
	}
	if !launchCalled {
		t.Fatalf("expected relaunch to be called for non-restartable created container")
	}
	if item.RuntimeState != "running" || item.State != "running" {
		t.Fatalf("expected running state after recreate, got runtime=%q state=%q", item.RuntimeState, item.State)
	}
	if item.FailureCount != 0 {
		t.Fatalf("expected failure count reset after recreate, got %d", item.FailureCount)
	}
	if item.LastError != "" {
		t.Fatalf("expected last error cleared after recreate, got %q", item.LastError)
	}
}

func TestReconcileLocalDesiredRunning_CreatedTransientStartErrorBumpsFailure(t *testing.T) {
	openLocalReconcileTestDB(t)

	pm := &ProcessManager{}
	pm.logger = logging.NewModuleLogger("test-process-manager")
	item := &models.LocalDeployedMicroservice{
		LocalUUID:    "local-2",
		Generation:   1,
		RuntimeState: "created",
		State:        "created",
		DesiredState: "running",
	}
	container := &engine.Container{ID: "container-2"}
	pm.startMicroserviceFn = func(_ string) error { return errors.New("temporary start error") }
	pm.getContainerStatusFn = func(_, _ string) (*models.MicroserviceStatus, error) {
		return &models.MicroserviceStatus{Status: models.MicroserviceStateCreated}, nil
	}

	pm.reconcileLocalDesiredRunning(item, container, 456)

	if item.FailureCount != 1 {
		t.Fatalf("expected failure count incremented to 1, got %d", item.FailureCount)
	}
	if item.RuntimeState != "created" || item.State != "created" {
		t.Fatalf("expected created state retained for transient error, got runtime=%q state=%q", item.RuntimeState, item.State)
	}
}

func TestReconcileLocalDesiredRunning_CreatedNonRestartableRecreateLaunchFailureTracksFailure(t *testing.T) {
	openLocalReconcileTestDB(t)

	pm := &ProcessManager{}
	pm.logger = logging.NewModuleLogger("test-process-manager")
	item := &models.LocalDeployedMicroservice{
		LocalUUID:    "local-3",
		Generation:   1,
		RuntimeState: "created",
		State:        "created",
		DesiredState: "running",
	}
	container := &engine.Container{ID: "container-3"}
	pm.startMicroserviceFn = func(_ string) error {
		return &engine.NonRestartableContainerError{ContainerID: "container-3", Reason: "CONTAINER_EXITED", ExitCode: 255, Message: "terminal"}
	}
	pm.removeContainerByIDFn = func(_ string) error { return nil }
	pm.launchLocalDeploymentFn = func(target *models.LocalDeployedMicroservice, _ int64) {
		pm.bumpLocalFailure(target, errors.New("recreate launch failed"), "failed")
	}
	pm.getContainerStatusFn = func(_, _ string) (*models.MicroserviceStatus, error) {
		return &models.MicroserviceStatus{Status: models.MicroserviceStateCreated}, nil
	}

	pm.reconcileLocalDesiredRunning(item, container, 789)

	if item.FailureCount != 1 {
		t.Fatalf("expected failure count incremented after recreate launch failure, got %d", item.FailureCount)
	}
	if item.LastError == "" {
		t.Fatalf("expected last error set on recreate launch failure")
	}
}

func openLocalReconcileTestDB(t *testing.T) {
	t.Helper()
	db := store.GetInstance()
	_ = db.Close()
	if err := db.Open(t.TempDir()); err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
}

func minimalLocalManifestYAML() string {
	return `apiVersion: iofog.org/v3
kind: Microservice
metadata:
  name: local-ms
spec:
  images:
    arm64: busybox:latest
  container:
    rootHostAccess: false
`
}
