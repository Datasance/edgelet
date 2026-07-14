package processmanager

import (
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eclipse-iofog/edgelet/internal/models"
	"github.com/eclipse-iofog/edgelet/internal/runtimestate"
	"github.com/eclipse-iofog/edgelet/internal/store"
	"github.com/eclipse-iofog/edgelet/internal/utils/logging"
	"github.com/eclipse-iofog/edgelet/internal/workloadmeta"
	"github.com/eclipse-iofog/edgelet/pkg/engine"
)

func TestDrainRuntimeForDataPlaneStop_DelegatesToProcessManager(t *testing.T) {
	runtimestate.ResetForTests()
	runtimestate.GetState().SetEngineReady(true)
	t.Cleanup(runtimestate.ResetForTests)

	pm := &ProcessManager{
		engine:     newDrainTestEngine("c1"),
		engineName: "edgelet",
		logger:     logging.NewModuleLogger(ProcessManagerModuleName),
	}
	restore := SetInstanceForTest(pm)
	t.Cleanup(restore)
	t.Cleanup(func() { SetQuiesced(false) })

	if err := DrainRuntimeForDataPlaneStop(0); err != nil {
		t.Fatalf("expected drain success, got: %v", err)
	}
	if !IsQuiesced() {
		t.Fatal("expected reconcile to stay quiesced after data-plane drain")
	}
	if !IsQuiescedForDataPlaneDrain() {
		t.Fatal("expected data-plane drain quiesce hold")
	}
	if runtimestate.GetState().EngineReady() {
		t.Fatal("expected engineReady=false after data-plane drain")
	}
}

func TestDrainRuntimeForDataPlaneStop_NilEngineNoOp(t *testing.T) {
	runtimestate.ResetForTests()
	t.Cleanup(runtimestate.ResetForTests)
	t.Cleanup(func() { SetQuiesced(false) })

	pm := &ProcessManager{
		logger: logging.NewModuleLogger(ProcessManagerModuleName),
	}
	restore := SetInstanceForTest(pm)
	t.Cleanup(restore)

	if err := DrainRuntimeForDataPlaneStop(time.Second); err != nil {
		t.Fatalf("expected nil engine no-op, got: %v", err)
	}
	if !IsQuiescedForDataPlaneDrain() {
		t.Fatal("expected data-plane drain quiesce even when engine is nil")
	}
}

func TestTryResumeReconcileAfterDataPlaneEngineReady_ClearsHold(t *testing.T) {
	runtimestate.ResetForTests()
	runtimestate.GetState().SetEngineReady(false)
	t.Cleanup(runtimestate.ResetForTests)
	t.Cleanup(func() { SetQuiesced(false) })

	BeginQuiesceForDataPlaneDrain()
	TryResumeReconcileAfterDataPlaneEngineReady()

	if IsQuiesced() {
		t.Fatal("expected reconcile resumed after engine ready")
	}
	if IsQuiescedForDataPlaneDrain() {
		t.Fatal("expected data-plane drain hold cleared")
	}
	if !runtimestate.GetState().EngineReady() {
		t.Fatal("expected engineReady=true after resume")
	}
}

func TestTryResumeReconcileAfterDataPlaneEngineReady_NoOpWithoutHold(t *testing.T) {
	runtimestate.ResetForTests()
	runtimestate.GetState().SetEngineReady(false)
	t.Cleanup(runtimestate.ResetForTests)
	t.Cleanup(func() { SetQuiesced(false) })

	SetQuiesced(true)
	TryResumeReconcileAfterDataPlaneEngineReady()

	if !IsQuiesced() {
		t.Fatal("expected unrelated quiesce to remain active")
	}
	if runtimestate.GetState().EngineReady() {
		t.Fatal("expected engineReady unchanged without data-plane drain hold")
	}
}

func TestDrainRuntimeForDataPlaneShutdown_PreservesLocalWorkloadSpec(t *testing.T) {
	openLocalReconcileTestDB(t)

	const localUUID = "ms-c1"
	if err := store.GetInstance().UpsertLocalWorkload(&models.LocalDeployedMicroservice{
		LocalUUID:        localUUID,
		MicroserviceName: "runtime-spin-ms",
		ManifestYAML:     minimalLocalManifestYAML(),
		ImageName:        "ghcr.io/spinframework/containerd-shim-spin/examples/spin-rust-hello:v0.22.0",
		State:            "running",
		ContainerID:      "c1",
		DesiredState:     "running",
		RuntimeState:     "running",
	}); err != nil {
		t.Fatalf("upsert local workload: %v", err)
	}
	if err := store.GetInstance().UpsertRuntimeContainerRef(localUUID, store.RuntimeScopeLocal, "c1", "sandbox-1"); err != nil {
		t.Fatalf("upsert runtime ref: %v", err)
	}

	pm := &ProcessManager{
		engine:     newDrainTestEngine("c1"),
		engineName: "edgelet",
		logger:     logging.NewModuleLogger(ProcessManagerModuleName),
	}
	restore := SetInstanceForTest(pm)
	t.Cleanup(restore)

	if err := pm.DrainRuntimeForDataPlaneShutdown(0); err != nil {
		t.Fatalf("expected drain success, got: %v", err)
	}

	item, err := store.GetInstance().GetLocalWorkload(localUUID)
	if err != nil {
		t.Fatalf("expected local workload row to remain, got: %v", err)
	}
	if item.DesiredState != "running" {
		t.Fatalf("expected desired_state=running, got %q", item.DesiredState)
	}
	if item.ContainerID != "" {
		t.Fatalf("expected container_id cleared, got %q", item.ContainerID)
	}
	if item.RuntimeState != "pending" {
		t.Fatalf("expected runtime_state=pending, got %q", item.RuntimeState)
	}
	ref, err := store.GetInstance().GetRuntimeContainerRef(localUUID, store.RuntimeScopeLocal)
	if err != nil {
		t.Fatalf("get runtime ref: %v", err)
	}
	if ref != nil {
		t.Fatal("expected runtime_container_refs cleared after data-plane drain")
	}
}

func TestDrainRuntimeForDataPlaneShutdown_UsesFullCRITeardown(t *testing.T) {
	eng := newDrainTestEngine("c1")
	pm := &ProcessManager{
		engine:     eng,
		engineName: "edgelet",
		logger:     logging.NewModuleLogger(ProcessManagerModuleName),
	}

	if err := pm.DrainRuntimeForDataPlaneShutdown(0); err != nil {
		t.Fatalf("expected drain success, got: %v", err)
	}
	if got := atomic.LoadInt64(&eng.removeCalls); got == 0 {
		t.Fatal("expected RemoveContainer during data-plane drain")
	}
	if got := atomic.LoadInt64(&eng.stopCalls); got == 0 {
		t.Fatal("expected StopContainer before RemoveContainer during data-plane drain")
	}
}

func TestDrainRuntimeForDataPlaneShutdown_TimeoutHasRemainingIDs(t *testing.T) {
	eng := newDrainTestEngine("c3", "c1", "c2")
	eng.removeRemoves = false
	eng.stopRemoves = false
	pm := &ProcessManager{
		engine:     eng,
		engineName: "edgelet",
		logger:     logging.NewModuleLogger(ProcessManagerModuleName),
	}

	err := pm.DrainRuntimeForDataPlaneShutdown(10 * time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	wantSorted := "c1,c2,c3"
	if !strings.Contains(err.Error(), wantSorted) {
		t.Fatalf("expected sorted remaining IDs %q in error, got %q", wantSorted, err.Error())
	}
}

func TestDrainRuntimeForDataPlaneShutdown_PreservesControllerSpec(t *testing.T) {
	openLocalReconcileTestDB(t)

	const msUUID = "ms-ctrl-1"
	const containerID = "c-ctrl"
	if err := store.GetInstance().SaveControllerMicroservices([]*models.Microservice{{
		MicroserviceUUID: msUUID,
		ImageName:        "alpine:3.19",
		ContainerID:      containerID,
		MicroserviceName: "pot-ms",
	}}); err != nil {
		t.Fatalf("save controller microservice: %v", err)
	}
	if err := store.GetInstance().UpsertRuntimeContainerRef(msUUID, store.RuntimeScopeController, containerID, "sandbox-1"); err != nil {
		t.Fatalf("upsert runtime ref: %v", err)
	}

	eng := newDrainTestEngine(containerID)
	eng.running[containerID] = engine.Container{
		ID: containerID,
		Labels: map[string]string{
			workloadmeta.LabelAppManagedBy:    workloadmeta.ManagedByValue,
			workloadmeta.LabelMicroserviceUID: msUUID,
		},
	}
	pm := &ProcessManager{
		engine:     eng,
		engineName: "edgelet",
		logger:     logging.NewModuleLogger(ProcessManagerModuleName),
	}

	if err := pm.DrainRuntimeForDataPlaneShutdown(0); err != nil {
		t.Fatalf("expected drain success, got: %v", err)
	}

	loaded, err := store.GetInstance().LoadControllerMicroservices()
	if err != nil {
		t.Fatalf("load controller microservices: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected controller spec row to remain, got %d rows", len(loaded))
	}
	if loaded[0].ImageName != "alpine:3.19" {
		t.Fatal("expected controller spec image to remain")
	}
	if loaded[0].ContainerID != "" {
		t.Fatalf("expected controller container_id cleared, got %q", loaded[0].ContainerID)
	}
	ref, err := store.GetInstance().GetRuntimeContainerRef(msUUID, store.RuntimeScopeController)
	if err != nil {
		t.Fatalf("get runtime ref: %v", err)
	}
	if ref != nil {
		t.Fatal("expected controller runtime_container_refs cleared after data-plane drain")
	}
}
