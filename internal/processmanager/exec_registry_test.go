package processmanager

import (
	"context"
	"errors"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"strings"

	"github.com/eclipse-iofog/edgelet/internal/containerexec"
	"github.com/eclipse-iofog/edgelet/internal/models"
	"github.com/eclipse-iofog/edgelet/internal/utils/logging"
	"github.com/eclipse-iofog/edgelet/pkg/engine"
)

func TestExecSessionRegistry_RegisterReleaseOwnerMatch(t *testing.T) {
	reg := NewExecSessionRegistry()
	rec := &ExecSessionRecord{
		SessionID:     "session-1",
		MSUUID:        "ms-1",
		ContainerID:   "container-abc123456789",
		RuntimeExecID: "container-abc-exec-session",
		Owner:         ExecOwnerController,
	}
	if err := reg.Register(rec); err != nil {
		t.Fatalf("register: %v", err)
	}
	if _, err := reg.Release("session-1", ExecOwnerLocal); !errors.Is(err, ErrExecSessionOwnerMismatch) {
		t.Fatalf("expected owner mismatch, got %v", err)
	}
	released, err := reg.Release("session-1", ExecOwnerController)
	if err != nil {
		t.Fatalf("release: %v", err)
	}
	if released.RuntimeExecID != rec.RuntimeExecID {
		t.Fatalf("unexpected runtime exec id: %s", released.RuntimeExecID)
	}
	if _, ok := reg.Get("session-1"); ok {
		t.Fatal("expected session removed")
	}
}

func TestExecSessionRegistry_ConcurrentLocalSessionsDistinctRuntimeIDs(t *testing.T) {
	containerID := "0123456789abcdef0123456789abcdef"
	reg := NewExecSessionRegistry()
	id1 := runtimeExecIDLocal(containerID, "local-aaaa-bbbb-cccc")
	id2 := runtimeExecIDLocal(containerID, "local-dddd-eeee-ffff")
	if id1 == id2 {
		t.Fatalf("expected distinct runtime ids: %q %q", id1, id2)
	}
	if err := reg.Register(&ExecSessionRecord{SessionID: "local-aaaa-bbbb-cccc", MSUUID: "ms-1", ContainerID: containerID, RuntimeExecID: id1, Owner: ExecOwnerLocal}); err != nil {
		t.Fatalf("register 1: %v", err)
	}
	if err := reg.Register(&ExecSessionRecord{SessionID: "local-dddd-eeee-ffff", MSUUID: "ms-1", ContainerID: containerID, RuntimeExecID: id2, Owner: ExecOwnerLocal}); err != nil {
		t.Fatalf("register 2: %v", err)
	}
	if len(reg.ListInteractiveForMS("ms-1")) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(reg.ListInteractiveForMS("ms-1")))
	}
}

func TestRuntimeExecIDPatterns(t *testing.T) {
	cid := "0123456789abcdef0123456789abcdef"
	if got := runtimeExecIDController(cid, "session-uuid-1"); got != "0123456789ab-exec-session-" {
		t.Fatalf("controller id: %q", got)
	}
	if got := runtimeExecIDLocal(cid, "local-uuid-2"); got != "0123456789ab-local-local-uu" {
		t.Fatalf("local id: %q", got)
	}
}

type mockExecCallback struct {
	onError func(error)
}

func (m *mockExecCallback) GetStdinReader() io.Reader  { return nil }
func (m *mockExecCallback) GetStdoutWriter() io.Writer { return io.Discard }
func (m *mockExecCallback) GetStderrWriter() io.Writer { return io.Discard }
func (m *mockExecCallback) OnComplete()                {}
func (m *mockExecCallback) OnError(err error) {
	if m.onError != nil {
		m.onError(err)
	}
}
func (m *mockExecCallback) IsRunning() bool { return true }

type execTestEngine struct {
	createCalls       atomic.Int32
	startCalls        atomic.Int32
	stopCalls         atomic.Int32
	sweepCalls        atomic.Int32
	running           atomic.Bool
	startDelay        time.Duration
	container         *engine.Container
	assignedExecID    string
	startNeverRunning bool
	lastStartExecID   atomic.Value // string
}

func (e *execTestEngine) Init(_ engine.EngineConfig) error { return nil }
func (e *execTestEngine) Close() error                     { return nil }
func (e *execTestEngine) GetContainer(_ string) (*engine.Container, error) {
	return e.container, nil
}
func (e *execTestEngine) GetContainerByID(id string) (*engine.Container, error) {
	if e.container != nil && e.container.ID == id {
		return e.container, nil
	}
	return e.container, nil
}
func (e *execTestEngine) GetContainerSandboxID(_ string) (string, error) { return "", nil }
func (e *execTestEngine) GetRunningContainers() ([]engine.Container, error) {
	return nil, nil
}
func (e *execTestEngine) GetAllContainers() ([]engine.Container, error) { return nil, nil }
func (e *execTestEngine) CreateContainer(_ *models.Microservice, _ string) (string, error) {
	return "", errors.New("not implemented")
}
func (e *execTestEngine) StartContainer(_ string) error          { return nil }
func (e *execTestEngine) StopContainer(_ string) error           { return nil }
func (e *execTestEngine) KillContainer(_ string) error           { return nil }
func (e *execTestEngine) RemoveContainer(_ string, _ bool) error { return nil }
func (e *execTestEngine) PullImage(_ string, _ *models.Registry, _ *engine.PullImageOptions) error {
	return errors.New("not implemented")
}
func (e *execTestEngine) FindLocalImage(_, _ string, _ bool) (bool, error) { return false, nil }
func (e *execTestEngine) RemoveImage(_ string) error                       { return nil }
func (e *execTestEngine) PruneImages() error                               { return nil }
func (e *execTestEngine) GetContainerStatus(_, _ string) (*models.MicroserviceStatus, error) {
	return nil, errors.New("not implemented")
}
func (e *execTestEngine) GetContainerStats(_ string) (*engine.ContainerStats, error) {
	return nil, errors.New("not implemented")
}
func (e *execTestEngine) GetContainerIPAddress(_ string) (string, error) { return "", nil }
func (e *execTestEngine) GetContainerStartedAt(_ string) (int64, error)  { return 0, nil }
func (e *execTestEngine) TailContainerLogs(_, _, _ string, _ engine.LogTailHandler, _ *engine.TailConfig) error {
	return errors.New("not implemented")
}
func (e *execTestEngine) AreMicroserviceAndContainerEqual(_ string, _ *models.Microservice, _ *models.Registry) bool {
	return false
}
func (e *execTestEngine) EnsureNetwork(_ string) error { return nil }
func (e *execTestEngine) CreateExecSession(_ string, runtimeExecID string, _ []string) (string, error) {
	e.createCalls.Add(1)
	if runtimeExecID == "" {
		return "", errors.New("runtime exec id required")
	}
	if id := strings.TrimSpace(e.assignedExecID); id != "" {
		return id, nil
	}
	return runtimeExecID, nil
}
func (e *execTestEngine) StartExecSession(execID string, _ io.Reader, _, _ io.Writer) error {
	e.startCalls.Add(1)
	e.lastStartExecID.Store(execID)
	if e.startDelay > 0 {
		time.Sleep(e.startDelay)
	}
	if !e.startNeverRunning {
		e.running.Store(true)
	}
	return nil
}
func (e *execTestEngine) GetExecSessionStatus(_ string) (bool, error) {
	return e.running.Load(), nil
}
func (e *execTestEngine) GetExecSessionExitCode(_ string) (int, error) {
	return 0, errors.New("unavailable")
}
func (e *execTestEngine) ResizeExecSession(_ string, _, _ uint32) error {
	return errors.New("not implemented")
}
func (e *execTestEngine) StopExecSession(_ string) error {
	e.stopCalls.Add(1)
	e.running.Store(false)
	return nil
}
func (e *execTestEngine) GetContainerMicroserviceUUID(_ engine.Container) string { return "" }
func (e *execTestEngine) GetContainerName(_ engine.Container) string             { return "" }
func (e *execTestEngine) ListImages(_ context.Context) ([]engine.ImageInfo, error) {
	return nil, errors.New("not implemented")
}
func (e *execTestEngine) LoadImageFromPath(_ context.Context, _ string) ([]engine.LoadedImage, error) {
	return nil, errors.New("not implemented")
}
func (e *execTestEngine) DeleteImage(_ context.Context, _ string) error {
	return errors.New("not implemented")
}
func (e *execTestEngine) PruneDangling(_ context.Context) (*engine.ImagePruneReport, error) {
	return nil, errors.New("not implemented")
}
func (e *execTestEngine) PruneContainers(_ context.Context) (*engine.ContainerPruneReport, error) {
	return nil, errors.New("not implemented")
}
func (e *execTestEngine) PruneVolumes(_ context.Context) (*engine.VolumePruneReport, error) {
	return nil, errors.New("not implemented")
}
func (e *execTestEngine) RemoveNamedVolume(_ context.Context, _ string) error { return nil }
func (e *execTestEngine) InspectContainerRaw(_ string) (map[string]any, error) {
	return nil, errors.New("not implemented")
}
func (e *execTestEngine) SweepOrphanExecSessions(_ string, _ map[string]struct{}) error {
	e.sweepCalls.Add(1)
	return nil
}

func newExecTestProcessManager(eng *execTestEngine) *ProcessManager {
	eng.container = &engine.Container{ID: "0123456789abcdef0123456789abcdef"}
	pm := &ProcessManager{
		engine:       eng,
		execRegistry: NewExecSessionRegistry(),
		logger:       logging.NewModuleLogger(ProcessManagerModuleName),
	}
	pm.containerManager = NewContainerManager(eng, nil, "edgelet")
	return pm
}

func TestExecSessionRegistry_SetRuntimeExecID(t *testing.T) {
	reg := NewExecSessionRegistry()
	_ = reg.Register(&ExecSessionRecord{
		SessionID:     "sess-1",
		MSUUID:        "ms-1",
		ContainerID:   "cid",
		RuntimeExecID: "hint-id",
		Owner:         ExecOwnerController,
	})
	reg.SetRuntimeExecID("sess-1", "engine-assigned-id")
	rec, ok := reg.Get("sess-1")
	if !ok || rec.RuntimeExecID != "engine-assigned-id" {
		t.Fatalf("expected engine-assigned-id, got %+v ok=%v", rec, ok)
	}
}

func TestCreateLocalExecSession_SyncStartGate(t *testing.T) {
	eng := &execTestEngine{startDelay: 200 * time.Millisecond}
	pm := newExecTestProcessManager(eng)

	start := time.Now()
	if err := pm.CreateLocalExecSession("local-session-1", "ms-1", containerexec.ShellCommandInteractive(), &mockExecCallback{}); err != nil {
		t.Fatalf("create local exec: %v", err)
	}
	if elapsed := time.Since(start); elapsed < 150*time.Millisecond {
		t.Fatalf("expected sync start gate to wait for running, elapsed=%v", elapsed)
	}
	rec, ok := pm.GetSession("local-session-1")
	if !ok || !rec.Started {
		t.Fatalf("expected started session record: %+v ok=%v", rec, ok)
	}
}

func TestCreateLocalExecSession_OneShotSkipsStartGate(t *testing.T) {
	SetExecStartGateTimeoutForTest(200 * time.Millisecond)
	t.Cleanup(ResetExecStartGateTimeoutForTest)

	eng := &execTestEngine{startNeverRunning: true}
	pm := newExecTestProcessManager(eng)

	start := time.Now()
	err := pm.CreateLocalExecSession("local-oneshot", "ms-1", []string{"nslookup", "edgelet.local-dns-b"}, &mockExecCallback{})
	if err != nil {
		t.Fatalf("one-shot exec should skip running gate: %v", err)
	}
	if elapsed := time.Since(start); elapsed >= 150*time.Millisecond {
		t.Fatalf("one-shot should return immediately, elapsed=%v", elapsed)
	}
	rec, ok := pm.GetSession("local-oneshot")
	if !ok {
		t.Fatal("expected registry row for one-shot session")
	}
	if rec.Started {
		t.Fatal("one-shot session should not wait for MarkStarted")
	}
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) && eng.startCalls.Load() == 0 {
		time.Sleep(10 * time.Millisecond)
	}
	if eng.startCalls.Load() != 1 {
		t.Fatalf("expected StartExecSession once, got %d", eng.startCalls.Load())
	}
}

func TestCreateLocalExecSession_InteractiveStillUsesStartGate(t *testing.T) {
	SetExecStartGateTimeoutForTest(200 * time.Millisecond)
	t.Cleanup(ResetExecStartGateTimeoutForTest)

	eng := &execTestEngine{startNeverRunning: true}
	pm := newExecTestProcessManager(eng)

	err := pm.CreateLocalExecSession("local-interactive", "ms-1", containerexec.ShellCommandInteractive(), &mockExecCallback{})
	if !errors.Is(err, ErrExecStartTimeout) {
		t.Fatalf("expected start timeout for interactive shell, got %v", err)
	}
	if _, ok := pm.GetSession("local-interactive"); ok {
		t.Fatal("expected registry row removed on interactive timeout")
	}
}

func TestCreateControllerExecSession_StartTimeoutReleasesRegistry(t *testing.T) {
	SetExecStartGateTimeoutForTest(200 * time.Millisecond)
	t.Cleanup(ResetExecStartGateTimeoutForTest)

	eng := &execTestEngine{startNeverRunning: true}
	pm := newExecTestProcessManager(eng)

	err := pm.CreateControllerExecSession("ctrl-session-1", "ms-1", []string{"sh"}, &mockExecCallback{})
	if !errors.Is(err, ErrExecStartTimeout) {
		t.Fatalf("expected start timeout, got %v", err)
	}
	if _, ok := pm.GetSession("ctrl-session-1"); ok {
		t.Fatal("expected registry row removed on timeout")
	}
	if eng.stopCalls.Load() == 0 {
		t.Fatal("expected StopExecSession on timeout")
	}
}

func TestCreateControllerExecSession_UsesEngineAssignedExecID(t *testing.T) {
	const dockerExecID = "5dcf7ebdc4eebc12cf6971eea8968e198c4a72a3981a0f7544b0ead8b411bfa2"
	eng := &execTestEngine{
		assignedExecID: dockerExecID,
		startDelay:     50 * time.Millisecond,
	}
	pm := newExecTestProcessManager(eng)
	sessionID := "7cce7c2f-01d7-4503-bf77-ce70ce447afe"
	if err := pm.CreateControllerExecSession(sessionID, "ms-1", []string{"sh"}, &mockExecCallback{}); err != nil {
		t.Fatalf("create controller exec: %v", err)
	}
	rec, ok := pm.GetSession(sessionID)
	if !ok {
		t.Fatal("expected session in registry")
	}
	if rec.RuntimeExecID != dockerExecID {
		t.Fatalf("registry runtime exec id: got %q want %q", rec.RuntimeExecID, dockerExecID)
	}
	startedRaw := eng.lastStartExecID.Load()
	startedID, ok := startedRaw.(string)
	if !ok {
		t.Fatalf("expected string exec id, got %T", startedRaw)
	}
	if startedID != dockerExecID {
		t.Fatalf("StartExecSession id: got %q want %q", startedID, dockerExecID)
	}
	hint := runtimeExecIDController(eng.container.ID, sessionID)
	if rec.RuntimeExecID == hint {
		t.Fatalf("expected engine-assigned id, still using hint %q", hint)
	}
}

func TestReleaseExecSession_StopExecSession(t *testing.T) {
	eng := &execTestEngine{}
	pm := newExecTestProcessManager(eng)
	eng.running.Store(true)

	reg := pm.ensureExecRegistry()
	_ = reg.Register(&ExecSessionRecord{
		SessionID:     "s1",
		MSUUID:        "ms-1",
		ContainerID:   eng.container.ID,
		RuntimeExecID: runtimeExecIDController(eng.container.ID, "s1"),
		Owner:         ExecOwnerController,
		Started:       true,
	})
	if err := pm.ReleaseExecSession("s1", ExecOwnerController); err != nil {
		t.Fatalf("release: %v", err)
	}
	if eng.stopCalls.Load() != 1 {
		t.Fatalf("expected stop call, got %d", eng.stopCalls.Load())
	}
}

func TestStopAllInteractiveForMicroservice(t *testing.T) {
	eng := &execTestEngine{}
	pm := newExecTestProcessManager(eng)
	reg := pm.ensureExecRegistry()
	cid := eng.container.ID
	_ = reg.Register(&ExecSessionRecord{SessionID: "c1", MSUUID: "ms-1", ContainerID: cid, RuntimeExecID: "r1", Owner: ExecOwnerController})
	_ = reg.Register(&ExecSessionRecord{SessionID: "l1", MSUUID: "ms-1", ContainerID: cid, RuntimeExecID: "r2", Owner: ExecOwnerLocal})
	pm.StopAllInteractiveForMicroservice("ms-1")
	if eng.stopCalls.Load() != 2 {
		t.Fatalf("expected 2 stops, got %d", eng.stopCalls.Load())
	}
	if len(reg.ListInteractiveForMS("ms-1")) != 0 {
		t.Fatal("expected registry cleared")
	}
}

func TestHealthcheckNotInExecRegistry(t *testing.T) {
	reg := NewExecSessionRegistry()
	if _, ok := reg.Get("0123456789ab-hc-abc123"); ok {
		t.Fatal("healthcheck exec id must not be pre-registered")
	}
	if len(reg.RuntimeExecIDsForContainer("0123456789abcdef0123456789abcdef")) != 0 {
		t.Fatal("expected empty registry keep set")
	}
}

func TestCreateExecSessionOrphanSweep(t *testing.T) {
	eng := &execTestEngine{}
	pm := newExecTestProcessManager(eng)
	if _, err := pm.prepareAndCreateExec(eng.container.ID, runtimeExecIDLocal(eng.container.ID, "sess-a"), []string{"sh"}); err != nil {
		t.Fatalf("prepareAndCreateExec: %v", err)
	}
	if eng.sweepCalls.Load() != 1 {
		t.Fatalf("expected orphan sweep call, got %d", eng.sweepCalls.Load())
	}
}

func TestLegacyCreateExecSession_UsesNonDeterministicRuntimeID(t *testing.T) {
	eng := &execTestEngine{}
	pm := newExecTestProcessManager(eng)
	runtimeID, err := pm.CreateExecSession("ms-1", []string{"sh"}, &mockExecCallback{})
	if err != nil {
		t.Fatalf("CreateExecSession: %v", err)
	}
	if runtimeID == eng.container.ID+"-exec" {
		t.Fatalf("legacy path must not use deterministic %q-exec id", eng.container.ID)
	}
	if !strings.Contains(runtimeID, "-exec-") {
		t.Fatalf("expected controller-style runtime id, got %q", runtimeID)
	}
}
