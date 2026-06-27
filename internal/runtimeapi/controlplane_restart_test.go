package runtimeapi

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/eclipse-iofog/edgelet/internal/models"
	"github.com/eclipse-iofog/edgelet/internal/processmanager"
	"github.com/eclipse-iofog/edgelet/internal/store"
	"github.com/eclipse-iofog/edgelet/internal/workloadmeta"
	"github.com/eclipse-iofog/edgelet/pkg/engine"
)

func testControlPlaneManifestYAML() string {
	return `apiVersion: edgelet.iofog.org/v1
kind: ControlPlane
metadata:
  name: pot
  namespace: default
spec:
  controller:
    image: ghcr.io/datasance/controller:3.8.0-beta.0
  auth:
    mode: embedded
    bootstrap:
      username: admin
      password: AdminPass123!
`
}

type cpRestartTestEngine struct {
	containerID      string
	microserviceUUID string
}

func (e *cpRestartTestEngine) Init(engine.EngineConfig) error { return nil }
func (e *cpRestartTestEngine) Close() error                   { return nil }

func (e *cpRestartTestEngine) GetContainer(msUUID string) (*engine.Container, error) {
	if strings.TrimSpace(msUUID) == strings.TrimSpace(e.microserviceUUID) && e.containerID != "" {
		return e.containerByID(e.containerID), nil
	}
	return nil, nil
}

func (e *cpRestartTestEngine) GetContainerByID(id string) (*engine.Container, error) {
	if id == e.containerID {
		return e.containerByID(id), nil
	}
	return nil, nil
}

func (e *cpRestartTestEngine) containerByID(id string) *engine.Container {
	return &engine.Container{
		ID:    id,
		Image: "ghcr.io/datasance/controller:3.8.0-beta.0",
		Labels: map[string]string{
			workloadmeta.LabelMicroserviceUID: e.microserviceUUID,
		},
	}
}

func (e *cpRestartTestEngine) GetContainerSandboxID(string) (string, error) { return "sandbox-1", nil }
func (e *cpRestartTestEngine) GetRunningContainers() ([]engine.Container, error) {
	return nil, nil
}
func (e *cpRestartTestEngine) GetAllContainers() ([]engine.Container, error) { return nil, nil }
func (e *cpRestartTestEngine) CreateContainer(*models.Microservice, string) (string, error) {
	return "", nil
}
func (e *cpRestartTestEngine) StartContainer(string) error { return nil }
func (e *cpRestartTestEngine) StopContainer(string) error  { return nil }
func (e *cpRestartTestEngine) KillContainer(string) error {
	return nil
}
func (e *cpRestartTestEngine) RemoveContainer(string, bool) error { return nil }
func (e *cpRestartTestEngine) PullImage(string, *models.Registry, *engine.PullImageOptions) error {
	return nil
}
func (e *cpRestartTestEngine) FindLocalImage(string, string, bool) (bool, error) { return true, nil }
func (e *cpRestartTestEngine) RemoveImage(string) error                          { return nil }
func (e *cpRestartTestEngine) PruneImages() error                                { return nil }
func (e *cpRestartTestEngine) ListImages(context.Context) ([]engine.ImageInfo, error) {
	return nil, nil
}
func (e *cpRestartTestEngine) LoadImageFromPath(context.Context, string) ([]engine.LoadedImage, error) {
	return nil, nil
}
func (e *cpRestartTestEngine) DeleteImage(context.Context, string) error { return nil }
func (e *cpRestartTestEngine) PruneDangling(context.Context) (*engine.ImagePruneReport, error) {
	return nil, nil
}
func (e *cpRestartTestEngine) PruneContainers(context.Context) (*engine.ContainerPruneReport, error) {
	return nil, nil
}
func (e *cpRestartTestEngine) PruneVolumes(context.Context) (*engine.VolumePruneReport, error) {
	return nil, nil
}
func (e *cpRestartTestEngine) RemoveNamedVolume(context.Context, string) error { return nil }
func (e *cpRestartTestEngine) GetContainerStatus(string, string) (*models.MicroserviceStatus, error) {
	return &models.MicroserviceStatus{Status: models.MicroserviceStateRunning}, nil
}
func (e *cpRestartTestEngine) GetContainerStats(string) (*engine.ContainerStats, error) {
	return nil, nil
}
func (e *cpRestartTestEngine) GetContainerIPAddress(string) (string, error) { return "10.0.0.2", nil }
func (e *cpRestartTestEngine) GetContainerStartedAt(string) (int64, error)  { return 0, nil }
func (e *cpRestartTestEngine) InspectContainerRaw(string) (map[string]any, error) {
	return nil, nil
}
func (e *cpRestartTestEngine) TailContainerLogs(string, string, string, engine.LogTailHandler, *engine.TailConfig) error {
	return nil
}
func (e *cpRestartTestEngine) AreMicroserviceAndContainerEqual(string, *models.Microservice, *models.Registry) bool {
	return false
}
func (e *cpRestartTestEngine) EnsureNetwork(string) error { return nil }
func (e *cpRestartTestEngine) CreateExecSession(string, string, []string) (string, error) {
	return "", nil
}
func (e *cpRestartTestEngine) StartExecSession(string, io.Reader, io.Writer, io.Writer) error {
	return nil
}
func (e *cpRestartTestEngine) GetExecSessionStatus(string) (bool, error)  { return false, nil }
func (e *cpRestartTestEngine) GetExecSessionExitCode(string) (int, error) { return 0, nil }
func (e *cpRestartTestEngine) ResizeExecSession(string, uint32, uint32) error {
	return nil
}
func (e *cpRestartTestEngine) StopExecSession(string) error { return nil }
func (e *cpRestartTestEngine) GetContainerMicroserviceUUID(engine.Container) string {
	return e.microserviceUUID
}
func (e *cpRestartTestEngine) GetContainerName(engine.Container) string { return "controller" }

var _ engine.ContainerEngine = (*cpRestartTestEngine)(nil)

func setupControlPlaneRestartFacadeTest(t *testing.T, uuid string) *Facade {
	t.Helper()

	f := NewFacade()
	if err := f.db.Open(t.TempDir()); err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() {
		_ = f.db.Close()
		f.sr.ResetProcessManagerStatus()
	})

	eng := &cpRestartTestEngine{
		containerID:      "old-cid",
		microserviceUUID: uuid,
	}
	processmanager.ConfigureControlPlaneRestartForTest(eng, "edgelet", func(item *models.ControlPlaneDeployment, _ bool, now int64) error {
		item.RuntimeState = "running"
		item.State = "running"
		item.LastTransitionAt = now
		item.LastError = ""
		return store.GetInstance().UpsertSystemControlPlane(item)
	})
	t.Cleanup(processmanager.ResetProcessManagerEngineForTest)

	return f
}

func TestFacadeRestartControlPlane_HappyPathUnprovisioned(t *testing.T) {
	const uuid = "cp-restart-happy"
	f := setupControlPlaneRestartFacadeTest(t, uuid)
	dep := &models.ControlPlaneDeployment{
		ControllerUUID:     uuid,
		Namespace:          "default",
		Name:               "pot",
		ManifestYAML:       testControlPlaneManifestYAML(),
		Image:              "ghcr.io/datasance/controller:3.8.0-beta.0",
		DesiredState:       "running",
		RuntimeState:       "running",
		ContainerID:        "old-cid",
		Generation:         1,
		ObservedGeneration: 1,
	}
	if err := f.db.UpsertSystemControlPlane(dep); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	got, err := f.RestartControlPlane(false)
	if err != nil {
		t.Fatalf("RestartControlPlane: %v", err)
	}
	if got.RuntimeState != "running" {
		t.Fatalf("runtimeState: want running got %q", got.RuntimeState)
	}
}

func TestFacadeRestartControlPlane_NotFound(t *testing.T) {
	f := NewFacade()
	if err := f.db.Open(t.TempDir()); err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = f.db.Close() })

	_, err := f.RestartControlPlane(false)
	if !errors.Is(err, ErrControlPlaneNotFound) {
		t.Fatalf("expected ErrControlPlaneNotFound, got %v", err)
	}
}

func TestFacadeRestartControlPlane_RestartCountIncrements(t *testing.T) {
	const uuid = "cp-restart-count"
	f := setupControlPlaneRestartFacadeTest(t, uuid)
	dep := &models.ControlPlaneDeployment{
		ControllerUUID:     uuid,
		Namespace:          "default",
		Name:               "pot",
		ManifestYAML:       testControlPlaneManifestYAML(),
		DesiredState:       "running",
		RuntimeState:       "running",
		ContainerID:        "old-cid",
		RestartCount:       2,
		Generation:         1,
		ObservedGeneration: 1,
	}
	if err := f.db.UpsertSystemControlPlane(dep); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	got, err := f.RestartControlPlane(false)
	if err != nil {
		t.Fatalf("RestartControlPlane: %v", err)
	}
	if got.RestartCount != 3 {
		t.Fatalf("restartCount: want 3 got %d", got.RestartCount)
	}
}

func TestFacadeRestartControlPlane_ApplyInFlight(t *testing.T) {
	f := NewFacade()
	if err := f.db.Open(t.TempDir()); err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = f.db.Close() })

	nowSec := time.Now().Unix()
	dep := &models.ControlPlaneDeployment{
		ControllerUUID:     "cp-restart-blocked",
		Namespace:          "default",
		Name:               "pot",
		ManifestYAML:       testControlPlaneManifestYAML(),
		DesiredState:       "running",
		RuntimeState:       "starting",
		Generation:         2,
		ObservedGeneration: 1,
		LastStartAttemptAt: nowSec,
		LastTransitionAt:   nowSec,
	}
	if err := f.db.UpsertSystemControlPlane(dep); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	_, err := f.RestartControlPlane(false)
	if !IsControlPlaneRestartBlocked(err) {
		t.Fatalf("expected ErrControlPlaneRestartBlocked, got %v", err)
	}
}

func TestErrControlPlaneLifecycleBlocked_RestartMessage(t *testing.T) {
	err := &ErrControlPlaneLifecycleBlocked{Operation: "restart"}
	want := "controller microservice cannot be restarted via ms restart while agent is provisioned; use edgelet controlplane restart"
	if err.Error() != want {
		t.Fatalf("unexpected message: %q", err.Error())
	}
}

func TestFacadeGuardControlPlaneMicroserviceMutation_RestartPointsToControlPlaneRestart(t *testing.T) {
	f := NewFacade()
	if err := f.db.Open(t.TempDir()); err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = f.db.Close() })

	if err := f.db.UpsertSystemControlPlane(&models.ControlPlaneDeployment{
		ControllerUUID: "cp-restart-msg",
		Namespace:      "default",
		Name:           "pot",
		ManifestYAML:   testControlPlaneManifestYAML(),
		DesiredState:   "running",
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	f.fa.SetControllerStatus(models.ControllerStatusOK)

	_, err := f.RestartRuntimeMicroservice("cp-restart-msg")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "use edgelet controlplane restart") {
		t.Fatalf("unexpected message: %v", err)
	}
}
