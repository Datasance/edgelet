//revive:disable:nested-structs
package processmanager

import (
	"errors"
	"testing"

	"github.com/eclipse-iofog/edgelet/internal/models"
	"github.com/eclipse-iofog/edgelet/internal/store"
	"github.com/eclipse-iofog/edgelet/internal/utils/logging"
	"github.com/eclipse-iofog/edgelet/internal/workloadmeta"
	"github.com/eclipse-iofog/edgelet/pkg/engine"
)

type resolverTestMSM struct {
	lifecycleTestMSM
	ms *models.Microservice
}

func (m *resolverTestMSM) FindLatestMicroserviceByUUID(_ string) *models.Microservice {
	return m.ms
}

func TestResolveMicroserviceForLifecycle_Managed(t *testing.T) {
	pm := &ProcessManager{
		microserviceManager: &resolverTestMSM{
			ms: models.NewMicroservice("managed-1", "nginx:latest"),
		},
	}
	ms, err := pm.resolveMicroserviceForLifecycle("managed-1")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if ms.MicroserviceUUID != "managed-1" {
		t.Fatalf("unexpected uuid %q", ms.MicroserviceUUID)
	}
}

func TestResolveMicroserviceForLifecycle_Local(t *testing.T) {
	openLocalReconcileTestDB(t)
	pm := &ProcessManager{}
	if err := store.GetInstance().UpsertLocalWorkload(&models.LocalDeployedMicroservice{
		LocalUUID:    "local-resolve",
		ManifestYAML: minimalLocalManifestYAML(),
		DesiredState: "running",
	}); err != nil {
		t.Fatalf("upsert local: %v", err)
	}
	ms, err := pm.resolveMicroserviceForLifecycle("local-resolve")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if ms.MicroserviceUUID != "local-resolve" {
		t.Fatalf("unexpected uuid %q", ms.MicroserviceUUID)
	}
}

func TestResolveMicroserviceForLifecycle_NotFound(t *testing.T) {
	openLocalReconcileTestDB(t)
	pm := &ProcessManager{}
	_, err := pm.resolveMicroserviceForLifecycle("missing-uuid")
	if err == nil {
		t.Fatal("expected not found error")
	}
	if err.Error() != "microservice spec not found" {
		t.Fatalf("unexpected error: %v", err)
	}
}

type criLifecycleMSM struct {
	lifecycleTestMSM
	ms *models.Microservice
}

func (m *criLifecycleMSM) FindLatestMicroserviceByUUID(uuid string) *models.Microservice {
	if m.ms != nil && m.ms.MicroserviceUUID == uuid {
		return m.ms
	}
	return nil
}

type criLifecycleEngine struct {
	lifecycleTestEngine
	stopped             bool
	running             bool
	nonRestartableStart bool
	stopErr             error
	pullCalled          bool
	calls               struct {
		stop, start, remove, create int
	}
}

func (e *criLifecycleEngine) GetContainerStatus(string, string) (*models.MicroserviceStatus, error) {
	if e.running {
		return &models.MicroserviceStatus{Status: models.MicroserviceStateRunning}, nil
	}
	if e.stopped {
		msg := "CRI reason=CONTAINER_EXITED exitCode=143 message=container is in CONTAINER_EXITED state"
		return &models.MicroserviceStatus{Status: models.MicroserviceStateExiting, ErrorMessage: &msg}, nil
	}
	return &models.MicroserviceStatus{Status: models.MicroserviceStateCreated}, nil
}

func (e *criLifecycleEngine) StopContainer(string) error {
	e.calls.stop++
	if e.stopErr != nil {
		return e.stopErr
	}
	e.stopped = true
	e.running = false
	return nil
}

func (e *criLifecycleEngine) StartContainer(id string) error {
	e.calls.start++
	if e.nonRestartableStart && id == "cid-old" {
		return &engine.NonRestartableContainerError{
			ContainerID: id,
			Reason:      engine.CRIReasonContainerExited,
			ExitCode:    143,
			Message:     "container is in CONTAINER_EXITED state",
		}
	}
	e.running = true
	e.stopped = false
	return nil
}

func (e *criLifecycleEngine) RemoveContainer(string, bool) error {
	e.calls.remove++
	e.workload = nil
	return nil
}

func (e *criLifecycleEngine) CreateContainer(ms *models.Microservice, _ string) (string, error) {
	e.calls.create++
	e.createdID = "cid-new"
	e.running = false
	e.stopped = false
	e.workload = &engine.Container{
		ID:    e.createdID,
		Image: ms.ImageName,
		Labels: map[string]string{
			workloadmeta.LabelAppManagedBy:    workloadmeta.ManagedByValue,
			workloadmeta.LabelMicroserviceUID: ms.MicroserviceUUID,
		},
	}
	return e.createdID, nil
}

func (e *criLifecycleEngine) PullImage(string, *models.Registry, *engine.PullImageOptions) error {
	e.pullCalled = true
	return nil
}

func newCRILifecyclePM(t *testing.T, eng *criLifecycleEngine, engineName string, ms *models.Microservice) *ProcessManager {
	t.Helper()
	openLocalReconcileTestDB(t)
	pm := &ProcessManager{
		engine:              eng,
		engineName:          engineName,
		microserviceManager: &criLifecycleMSM{ms: ms, lifecycleTestMSM: lifecycleTestMSM{registry: &models.Registry{URL: "from_cache"}}},
		logger:              logging.NewModuleLogger("test-process-manager"),
	}
	pm.containerManager = NewContainerManager(eng, pm.microserviceManager, engineName)
	return pm
}

func TestRestartMicroservice_Docker_InPlace(t *testing.T) {
	eng := &criLifecycleEngine{
		lifecycleTestEngine: lifecycleTestEngine{
			workload: &engine.Container{
				ID: "cid-old",
				Labels: map[string]string{
					workloadmeta.LabelAppManagedBy:    workloadmeta.ManagedByValue,
					workloadmeta.LabelMicroserviceUID: "ms-docker",
				},
			},
		},
		running: true,
	}
	ms := models.NewMicroservice("ms-docker", "nginx:latest")
	ms.RegistryID = 1
	pm := newCRILifecyclePM(t, eng, "docker", ms)

	if err := pm.RestartMicroservice("ms-docker"); err != nil {
		t.Fatalf("RestartMicroservice: %v", err)
	}
	if eng.calls.remove != 0 || eng.calls.create != 0 {
		t.Fatalf("expected in-place restart, remove=%d create=%d", eng.calls.remove, eng.calls.create)
	}
	if eng.calls.stop == 0 || eng.calls.start == 0 {
		t.Fatalf("expected stop+start, stop=%d start=%d", eng.calls.stop, eng.calls.start)
	}
}

func TestRestartMicroservice_CRI_Recreates(t *testing.T) {
	eng := &criLifecycleEngine{
		lifecycleTestEngine: lifecycleTestEngine{
			workload: &engine.Container{
				ID: "cid-old",
				Labels: map[string]string{
					workloadmeta.LabelAppManagedBy:    workloadmeta.ManagedByValue,
					workloadmeta.LabelMicroserviceUID: "ms-cri",
				},
			},
		},
		running: true,
	}
	ms := models.NewMicroservice("ms-cri", "nginx:latest")
	ms.RegistryID = 1
	pm := newCRILifecyclePM(t, eng, "edgelet", ms)

	if err := pm.RestartMicroservice("ms-cri"); err != nil {
		t.Fatalf("RestartMicroservice: %v", err)
	}
	if eng.calls.stop == 0 || eng.calls.remove == 0 || eng.calls.create == 0 {
		t.Fatalf("expected stop+recreate, stop=%d remove=%d create=%d start=%d",
			eng.calls.stop, eng.calls.remove, eng.calls.create, eng.calls.start)
	}
	if eng.pullCalled {
		t.Fatal("expected no pull on CRI restart")
	}
}

func TestRestartMicroservice_CRI_StopFailure(t *testing.T) {
	eng := &criLifecycleEngine{
		lifecycleTestEngine: lifecycleTestEngine{
			workload: &engine.Container{
				ID: "cid-old",
				Labels: map[string]string{
					workloadmeta.LabelAppManagedBy:    workloadmeta.ManagedByValue,
					workloadmeta.LabelMicroserviceUID: "ms-stop-fail",
				},
			},
		},
		stopErr: errors.New("stop failed"),
	}
	ms := models.NewMicroservice("ms-stop-fail", "nginx:latest")
	ms.RegistryID = 1
	pm := newCRILifecyclePM(t, eng, "edgelet", ms)

	err := pm.RestartMicroservice("ms-stop-fail")
	if err == nil {
		t.Fatal("expected stop failure")
	}
	if eng.calls.remove != 0 || eng.calls.create != 0 {
		t.Fatal("expected no recreate after stop failure")
	}
}

func TestStartMicroservice_Docker_Exited(t *testing.T) {
	eng := &criLifecycleEngine{
		lifecycleTestEngine: lifecycleTestEngine{
			workload: &engine.Container{
				ID: "cid-old",
				Labels: map[string]string{
					workloadmeta.LabelAppManagedBy:    workloadmeta.ManagedByValue,
					workloadmeta.LabelMicroserviceUID: "ms-docker-exited",
				},
			},
		},
		stopped: true,
	}
	ms := models.NewMicroservice("ms-docker-exited", "nginx:latest")
	ms.RegistryID = 1
	pm := newCRILifecyclePM(t, eng, "docker", ms)

	if err := pm.StartMicroservice("ms-docker-exited"); err != nil {
		t.Fatalf("StartMicroservice: %v", err)
	}
	if eng.calls.start == 0 {
		t.Fatal("expected in-place start on docker")
	}
	if eng.calls.remove != 0 {
		t.Fatal("expected no recreate on docker exited start")
	}
}

func TestStartMicroservice_CRI_Exited(t *testing.T) {
	eng := &criLifecycleEngine{
		lifecycleTestEngine: lifecycleTestEngine{
			workload: &engine.Container{
				ID: "cid-old",
				Labels: map[string]string{
					workloadmeta.LabelAppManagedBy:    workloadmeta.ManagedByValue,
					workloadmeta.LabelMicroserviceUID: "ms-cri-exited",
				},
			},
		},
		stopped: true,
	}
	ms := models.NewMicroservice("ms-cri-exited", "nginx:latest")
	ms.RegistryID = 1
	pm := newCRILifecyclePM(t, eng, "edgelet", ms)

	if err := pm.StartMicroservice("ms-cri-exited"); err != nil {
		t.Fatalf("StartMicroservice: %v", err)
	}
	if eng.calls.remove == 0 || eng.calls.create == 0 {
		t.Fatalf("expected recreate on CRI exited start, remove=%d create=%d", eng.calls.remove, eng.calls.create)
	}
	if eng.pullCalled {
		t.Fatal("expected PullImage=false for exited start")
	}
}

func TestStartMicroservice_CRI_NonRestartableFallback(t *testing.T) {
	eng := &criLifecycleEngine{
		lifecycleTestEngine: lifecycleTestEngine{
			workload: &engine.Container{
				ID: "cid-old",
				Labels: map[string]string{
					workloadmeta.LabelAppManagedBy:    workloadmeta.ManagedByValue,
					workloadmeta.LabelMicroserviceUID: "ms-cri-fallback",
				},
			},
		},
		nonRestartableStart: true,
	}
	ms := models.NewMicroservice("ms-cri-fallback", "nginx:latest")
	ms.RegistryID = 1
	pm := newCRILifecyclePM(t, eng, "edgelet", ms)

	if err := pm.StartMicroservice("ms-cri-fallback"); err != nil {
		t.Fatalf("StartMicroservice: %v", err)
	}
	if eng.calls.start == 0 {
		t.Fatal("expected initial in-place start attempt")
	}
	if eng.calls.remove == 0 || eng.calls.create == 0 {
		t.Fatal("expected fallback recreate")
	}
}

func TestStartMicroservice_AlreadyRunning(t *testing.T) {
	eng := &criLifecycleEngine{
		lifecycleTestEngine: lifecycleTestEngine{
			workload: &engine.Container{
				ID: "cid-old",
				Labels: map[string]string{
					workloadmeta.LabelAppManagedBy:    workloadmeta.ManagedByValue,
					workloadmeta.LabelMicroserviceUID: "ms-running",
				},
			},
		},
		running: true,
	}
	ms := models.NewMicroservice("ms-running", "nginx:latest")
	ms.RegistryID = 1
	pm := newCRILifecyclePM(t, eng, "edgelet", ms)

	if err := pm.StartMicroservice("ms-running"); err != nil {
		t.Fatalf("StartMicroservice: %v", err)
	}
	if eng.calls.start != 0 || eng.calls.remove != 0 {
		t.Fatal("expected no-op for already running microservice")
	}
}

func TestStartMicroservice_NoContainer(t *testing.T) {
	openLocalReconcileTestDB(t)
	eng := &criLifecycleEngine{}
	pm := &ProcessManager{
		engine:     eng,
		engineName: "edgelet",
		logger:     logging.NewModuleLogger("test-process-manager"),
		microserviceManager: &criLifecycleMSM{
			ms:               models.NewMicroservice("ms-missing", "nginx:latest"),
			lifecycleTestMSM: lifecycleTestMSM{registry: &models.Registry{URL: "https://registry.example"}},
		},
	}
	pm.microserviceManager.(*criLifecycleMSM).ms.RegistryID = 1
	pm.containerManager = NewContainerManager(eng, pm.microserviceManager, "edgelet")

	if err := pm.StartMicroservice("ms-missing"); err != nil {
		t.Fatalf("StartMicroservice: %v", err)
	}
	if eng.calls.create == 0 {
		t.Fatal("expected create when container missing")
	}
	if !eng.pullCalled {
		t.Fatal("expected PullImage=true when container missing")
	}
}

func TestRestartMicroservice_UpdatesLocalDB(t *testing.T) {
	openLocalReconcileTestDB(t)
	eng := &criLifecycleEngine{
		lifecycleTestEngine: lifecycleTestEngine{
			workload: &engine.Container{
				ID: "cid-old",
				Labels: map[string]string{
					workloadmeta.LabelAppManagedBy:    workloadmeta.ManagedByValue,
					workloadmeta.LabelMicroserviceUID: "local-db",
				},
			},
		},
		running: true,
	}
	if err := store.GetInstance().UpsertLocalWorkload(&models.LocalDeployedMicroservice{
		LocalUUID:    "local-db",
		ManifestYAML: minimalLocalManifestYAML(),
		DesiredState: "running",
	}); err != nil {
		t.Fatalf("upsert local: %v", err)
	}
	pm := &ProcessManager{
		engine:     eng,
		engineName: "edgelet",
		logger:     logging.NewModuleLogger("test-process-manager"),
	}
	pm.containerManager = NewContainerManager(eng, &lifecycleTestMSM{registry: &models.Registry{URL: "from_cache"}}, "edgelet")

	if err := pm.RestartMicroservice("local-db"); err != nil {
		t.Fatalf("RestartMicroservice: %v", err)
	}
	item, err := store.GetInstance().GetLocalWorkload("local-db")
	if err != nil {
		t.Fatalf("get local: %v", err)
	}
	if item.ContainerID != "cid-new" {
		t.Fatalf("expected cid-new in local db, got %q", item.ContainerID)
	}
	if item.RuntimeState != "running" {
		t.Fatalf("expected running runtime state, got %q", item.RuntimeState)
	}
}
