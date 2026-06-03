package processmanager

import (
	"strings"
	"testing"

	"github.com/datasance/edgelet/internal/controlplane"
	"github.com/datasance/edgelet/internal/models"
	"github.com/datasance/edgelet/internal/store"
	"github.com/datasance/edgelet/internal/utils/logging"
	"github.com/datasance/edgelet/internal/workloadmeta"
	"github.com/datasance/edgelet/pkg/engine"
)

type controlPlaneCaptureEngine struct {
	lifecycleTestEngine
	lastMS *models.Microservice
}

func (e *controlPlaneCaptureEngine) CreateContainer(ms *models.Microservice, _ string) (string, error) {
	e.lastMS = ms
	return "cp-cid-" + ms.MicroserviceUUID, nil
}

func TestReconcileControlPlane_NoDeploymentIsNoOp(t *testing.T) {
	openLocalReconcileTestDB(t)
	pm := &ProcessManager{logger: logging.NewModuleLogger("test-process-manager")}
	pm.reconcileControlPlane()
}

func TestReconcileControlPlane_LaunchesMissingContainer(t *testing.T) {
	openLocalReconcileTestDB(t)

	eng := &controlPlaneCaptureEngine{}
	pm := &ProcessManager{
		logger:           logging.NewModuleLogger("test-process-manager"),
		engine:           eng,
		containerManager: NewContainerManager(eng, nil, "docker"),
	}

	dep := &models.ControlPlaneDeployment{
		ControllerUUID: "cp-launch-1",
		Namespace:      "default",
		Name:           "pot",
		ManifestYAML:   minimalControlPlaneManifestYAML(),
		DesiredState:   "running",
		Generation:     1,
	}
	if err := store.GetInstance().UpsertControlPlaneDeployment(dep); err != nil {
		t.Fatalf("upsert control plane: %v", err)
	}

	pm.reconcileControlPlane()

	got, found, err := store.GetInstance().GetControlPlaneDeployment()
	if err != nil || !found {
		t.Fatalf("get control plane: found=%v err=%v", found, err)
	}
	if got.RuntimeState != "running" {
		t.Fatalf("expected runtime_state=running, got %q", got.RuntimeState)
	}
	if got.ContainerID != "cp-cid-cp-launch-1" {
		t.Fatalf("expected container id cp-cid-cp-launch-1, got %q", got.ContainerID)
	}
	if eng.lastMS == nil {
		t.Fatal("expected CreateContainer to be called")
	}
	assertControlPlaneLaunchSpec(t, eng.lastMS)
}

func TestReconcileControlPlane_SkipsLaunchWhenApplyInFlight(t *testing.T) {
	openLocalReconcileTestDB(t)
	pm := &ProcessManager{logger: logging.NewModuleLogger("test-process-manager")}

	dep := &models.ControlPlaneDeployment{
		ControllerUUID:     "cp-inflight",
		Namespace:          "default",
		Name:               "pot",
		ManifestYAML:       minimalControlPlaneManifestYAML(),
		DesiredState:       "running",
		Generation:         2,
		ObservedGeneration: 0,
		RuntimeState:       "starting",
		LastStartAttemptAt: 9999999999,
	}
	if err := store.GetInstance().UpsertControlPlaneDeployment(dep); err != nil {
		t.Fatalf("upsert control plane: %v", err)
	}

	launchCalled := false
	pm.launchControlPlaneFn = func(*models.ControlPlaneDeployment, int64) {
		launchCalled = true
	}
	pm.reconcileControlPlane()

	if launchCalled {
		t.Fatal("expected reconcile to skip launch while apply is in-flight")
	}
}

func TestReconcileControlPlane_RecreatesOnGenerationBump(t *testing.T) {
	openLocalReconcileTestDB(t)

	eng := &controlPlaneCaptureEngine{}
	pm := &ProcessManager{
		logger:           logging.NewModuleLogger("test-process-manager"),
		engine:           eng,
		containerManager: NewContainerManager(eng, nil, "docker"),
	}

	dep := &models.ControlPlaneDeployment{
		ControllerUUID:     "cp-recreate",
		Namespace:          "default",
		Name:               "pot",
		ManifestYAML:       minimalControlPlaneManifestYAML(),
		DesiredState:       "running",
		Generation:         2,
		ObservedGeneration: 1,
		RuntimeState:       "running",
		ContainerID:        "old-cid",
	}
	if err := store.GetInstance().UpsertControlPlaneDeployment(dep); err != nil {
		t.Fatalf("upsert control plane: %v", err)
	}

	eng.workload = &engine.Container{
		ID:    "old-cid",
		Image: "ghcr.io/datasance/controller:3.7.0",
		Labels: map[string]string{
			workloadmeta.LabelMicroserviceUID: "cp-recreate",
		},
	}
	pm.getContainerStatusFn = func(_, _ string) (*models.MicroserviceStatus, error) {
		return &models.MicroserviceStatus{Status: models.MicroserviceStateRunning}, nil
	}

	pm.reconcileControlPlane()

	got, found, err := store.GetInstance().GetControlPlaneDeployment()
	if err != nil || !found {
		t.Fatalf("get control plane: found=%v err=%v", found, err)
	}
	if got.ObservedGeneration != 2 {
		t.Fatalf("expected observed generation 2, got %d", got.ObservedGeneration)
	}
	if got.ContainerID != "cp-cid-cp-recreate" {
		t.Fatalf("expected recreated container id, got %q", got.ContainerID)
	}
}

func TestBuildControlPlaneLaunchSpec(t *testing.T) {
	openLocalReconcileTestDB(t)
	pm := &ProcessManager{logger: logging.NewModuleLogger("test-process-manager")}

	item := &models.ControlPlaneDeployment{
		ControllerUUID: "cp-spec",
		ManifestYAML:   minimalControlPlaneManifestYAML(),
	}
	_, ms, _, err := pm.buildControlPlaneLaunchSpec(item)
	if err != nil {
		t.Fatalf("build launch spec: %v", err)
	}
	assertControlPlaneLaunchSpec(t, ms)
}

func assertControlPlaneLaunchSpec(t *testing.T, ms *models.Microservice) {
	t.Helper()
	if ms == nil {
		t.Fatal("microservice is nil")
	}
	if !ms.IsController || !ms.IsSystem {
		t.Fatal("expected controller system flags")
	}
	if ms.ApplicationName != "default" || ms.MicroserviceName != "pot" {
		t.Fatalf("unexpected identity application=%q name=%q", ms.ApplicationName, ms.MicroserviceName)
	}
	if len(ms.PortMappings) != 2 {
		t.Fatalf("expected 2 port mappings, got %d", len(ms.PortMappings))
	}
	if ms.PortMappings[0].Outside != controlplane.HostAPIPort || ms.PortMappings[1].Outside != controlplane.HostViewerPort {
		t.Fatalf("unexpected host ports: %+v", ms.PortMappings)
	}

	hasNetRaw := false
	for _, cap := range ms.CapAdd {
		if strings.EqualFold(cap, "NET_RAW") {
			hasNetRaw = true
		}
	}
	if !hasNetRaw {
		t.Fatal("expected NET_RAW in CapAdd")
	}

	volumes := map[string]string{}
	for _, vm := range ms.VolumeMappings {
		volumes[vm.HostDestination] = vm.ContainerDestination
	}
	if volumes[controlplane.VolumeDBName] != controlplane.ContainerDBPath {
		t.Fatalf("missing db volume mapping: %#v", volumes)
	}
	if volumes[controlplane.VolumeLogName] != controlplane.ContainerLogPath {
		t.Fatalf("missing log volume mapping: %#v", volumes)
	}

	in := workloadmeta.BuildInput{
		MicroserviceUUID: ms.MicroserviceUUID,
		MicroserviceName: ms.MicroserviceName,
		ApplicationName:  ms.ApplicationName,
		NodeUUID:         "node-1",
		RuntimeEngine:    workloadmeta.RuntimeEngineDocker,
		IsController:     true,
		IsSystem:         true,
	}
	labels := workloadmeta.BuildLabels(in)
	if labels[workloadmeta.LabelRole] != workloadmeta.RoleController {
		t.Fatalf("expected role=controller label, got %q", labels[workloadmeta.LabelRole])
	}
	if labels[workloadmeta.LabelAppPartOf] != "default" || labels[workloadmeta.LabelAppName] != "pot" {
		t.Fatalf("unexpected dns identity labels: %#v", labels)
	}
}

func minimalControlPlaneManifestYAML() string {
	return `apiVersion: edgelet.iofog.org/v1
kind: ControlPlane
metadata:
  name: pot
  namespace: default
spec:
  controller:
    image: ghcr.io/datasance/controller:3.7.0
`
}
