package processmanager

import (
	"errors"
	"testing"

	"github.com/datasance/edgelet/internal/models"
	"github.com/datasance/edgelet/internal/store"
	"github.com/datasance/edgelet/internal/utils/logging"
	"github.com/datasance/edgelet/pkg/engine"
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
	recreateCalled := false
	pm.startMicroserviceFn = func(_ string) error {
		return &engine.NonRestartableContainerError{
			ContainerID: "old-container",
			Reason:      "CONTAINER_EXITED",
			ExitCode:    255,
			Message:     "container is in CONTAINER_EXITED state",
		}
	}
	pm.recreateLocalDeploymentFn = func(target *models.LocalDeployedMicroservice, pullImage bool, _ int64) error {
		recreateCalled = true
		if pullImage {
			t.Fatalf("expected pullImage=false for created non-restartable recreate")
		}
		target.ContainerID = "new-container"
		target.RuntimeState = "running"
		target.State = "running"
		target.LastError = ""
		target.FailureCount = 0
		return nil
	}
	pm.getContainerStatusFn = func(_, _ string) (*models.MicroserviceStatus, error) {
		return &models.MicroserviceStatus{Status: models.MicroserviceStateCreated}, nil
	}

	pm.reconcileLocalDesiredRunning(item, container, 123)

	if !recreateCalled {
		t.Fatalf("expected recreate to be called for non-restartable created container")
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
	pm.recreateLocalDeploymentFn = func(target *models.LocalDeployedMicroservice, _ bool, _ int64) error {
		pm.bumpLocalFailure(target, errors.New("recreate launch failed"), "failed")
		return errors.New("recreate launch failed")
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

func TestReconcileLocalDesiredRunning_ExitingNonRestartableRecreates(t *testing.T) {
	openLocalReconcileTestDB(t)

	pm := &ProcessManager{}
	pm.logger = logging.NewModuleLogger("test-process-manager")
	item := &models.LocalDeployedMicroservice{
		LocalUUID:    "local-exiting",
		ManifestYAML: minimalLocalManifestYAML(),
		Generation:   1,
		RuntimeState: "exiting",
		State:        "exiting",
		ContainerID:  "old-container",
		DesiredState: "running",
	}
	container := &engine.Container{ID: "old-container"}
	recreateCalled := false
	pm.recreateLocalDeploymentFn = func(target *models.LocalDeployedMicroservice, pullImage bool, _ int64) error {
		recreateCalled = true
		if pullImage {
			t.Fatalf("expected pullImage=false for exiting non-restartable recreate")
		}
		target.ContainerID = "new-container"
		target.RuntimeState = "running"
		target.State = "running"
		target.LastError = ""
		target.FailureCount = 0
		return nil
	}
	criMsg := "CRI reason=CONTAINER_EXITED exitCode=143 message=container is in CONTAINER_EXITED state"
	pm.getContainerStatusFn = func(_, _ string) (*models.MicroserviceStatus, error) {
		return &models.MicroserviceStatus{
			Status:       models.MicroserviceStateExiting,
			ErrorMessage: &criMsg,
		}, nil
	}

	pm.reconcileLocalDesiredRunning(item, container, 999)

	if !recreateCalled {
		t.Fatal("expected recreate for exiting CONTAINER_EXITED state")
	}
	if item.RuntimeState != "running" || item.State != "running" {
		t.Fatalf("expected running state after recreate, got runtime=%q state=%q", item.RuntimeState, item.State)
	}
	if item.FailureCount != 0 {
		t.Fatalf("expected failure count reset after recreate, got %d", item.FailureCount)
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
	return `apiVersion: edgelet.iofog.org/v1
kind: Microservice
metadata:
  name: local-ms
spec:
  images:
    x86: busybox:latest
  container:
    hostNetworkMode: false
    isPrivileged: false
`
}
