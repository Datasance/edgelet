package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eclipse-iofog/edgelet/internal/containerexec"
	"github.com/eclipse-iofog/edgelet/internal/models"
	"github.com/eclipse-iofog/edgelet/internal/processmanager"
	"github.com/eclipse-iofog/edgelet/internal/store"
	"github.com/eclipse-iofog/edgelet/pkg/engine"
	"github.com/gorilla/websocket"
)

type execAPITestEngine struct {
	container  *engine.Container
	running    atomic.Bool
	stopCalls  []string
	stopMu     sync.Mutex
	startDelay time.Duration
	neverReady bool
	sweepCalls atomic.Int32
}

func (e *execAPITestEngine) Init(_ engine.EngineConfig) error { return nil }
func (e *execAPITestEngine) Close() error                     { return nil }
func (e *execAPITestEngine) GetContainer(msUUID string) (*engine.Container, error) {
	if e.container != nil {
		return e.container, nil
	}
	return nil, errors.New("container not found")
}
func (e *execAPITestEngine) GetContainerByID(id string) (*engine.Container, error) {
	if e.container != nil && e.container.ID == id {
		return e.container, nil
	}
	return e.container, nil
}
func (e *execAPITestEngine) GetContainerSandboxID(_ string) (string, error) { return "", nil }
func (e *execAPITestEngine) GetRunningContainers() ([]engine.Container, error) {
	return nil, nil
}
func (e *execAPITestEngine) GetAllContainers() ([]engine.Container, error) { return nil, nil }
func (e *execAPITestEngine) CreateContainer(_ *models.Microservice, _ string) (string, error) {
	return "", errors.New("not implemented")
}
func (e *execAPITestEngine) StartContainer(_ string) error          { return nil }
func (e *execAPITestEngine) StopContainer(_ string) error           { return nil }
func (e *execAPITestEngine) KillContainer(_ string) error           { return nil }
func (e *execAPITestEngine) RemoveContainer(_ string, _ bool) error { return nil }
func (e *execAPITestEngine) PullImage(_ string, _ *models.Registry, _ *engine.PullImageOptions) error {
	return errors.New("not implemented")
}
func (e *execAPITestEngine) FindLocalImage(_, _ string, _ bool) (bool, error) { return false, nil }
func (e *execAPITestEngine) RemoveImage(_ string) error                       { return nil }
func (e *execAPITestEngine) PruneImages() error                               { return nil }
func (e *execAPITestEngine) GetContainerStatus(_, _ string) (*models.MicroserviceStatus, error) {
	return nil, errors.New("not implemented")
}
func (e *execAPITestEngine) GetContainerStats(_ string) (*engine.ContainerStats, error) {
	return nil, errors.New("not implemented")
}
func (e *execAPITestEngine) GetContainerIPAddress(_ string) (string, error) { return "", nil }
func (e *execAPITestEngine) GetContainerStartedAt(_ string) (int64, error)  { return 0, nil }
func (e *execAPITestEngine) TailContainerLogs(_, _, _ string, _ engine.LogTailHandler, _ *engine.TailConfig) error {
	return errors.New("not implemented")
}
func (e *execAPITestEngine) AreMicroserviceAndContainerEqual(_ string, _ *models.Microservice, _ *models.Registry) bool {
	return false
}
func (e *execAPITestEngine) EnsureNetwork(_ string) error { return nil }
func (e *execAPITestEngine) CreateExecSession(_ string, runtimeExecID string, _ []string) (string, error) {
	if runtimeExecID == "" {
		return "", errors.New("runtime exec id required")
	}
	return runtimeExecID, nil
}
func (e *execAPITestEngine) StartExecSession(_ string, stdin io.Reader, _, _ io.Writer) error {
	if e.startDelay > 0 {
		time.Sleep(e.startDelay)
	}
	if !e.neverReady {
		e.running.Store(true)
	}
	if stdin != nil {
		buf := make([]byte, 1024)
		for {
			if _, err := stdin.Read(buf); err != nil {
				break
			}
		}
	}
	e.running.Store(false)
	return nil
}
func (e *execAPITestEngine) GetExecSessionStatus(_ string) (bool, error) {
	return e.running.Load(), nil
}
func (e *execAPITestEngine) GetExecSessionExitCode(_ string) (int, error) { return 0, nil }
func (e *execAPITestEngine) ResizeExecSession(_ string, _, _ uint32) error {
	return nil
}
func (e *execAPITestEngine) StopExecSession(runtimeExecID string) error {
	e.stopMu.Lock()
	e.stopCalls = append(e.stopCalls, runtimeExecID)
	e.stopMu.Unlock()
	e.running.Store(false)
	return nil
}
func (e *execAPITestEngine) GetContainerMicroserviceUUID(_ engine.Container) string { return "" }
func (e *execAPITestEngine) GetContainerName(_ engine.Container) string             { return "" }
func (e *execAPITestEngine) ListImages(_ context.Context) ([]engine.ImageInfo, error) {
	return nil, errors.New("not implemented")
}
func (e *execAPITestEngine) LoadImageFromPath(_ context.Context, _ string) ([]engine.LoadedImage, error) {
	return nil, errors.New("not implemented")
}
func (e *execAPITestEngine) DeleteImage(_ context.Context, _ string) error {
	return errors.New("not implemented")
}
func (e *execAPITestEngine) PruneDangling(_ context.Context) (*engine.ImagePruneReport, error) {
	return nil, errors.New("not implemented")
}
func (e *execAPITestEngine) PruneContainers(_ context.Context) (*engine.ContainerPruneReport, error) {
	return nil, errors.New("not implemented")
}
func (e *execAPITestEngine) PruneVolumes(_ context.Context) (*engine.VolumePruneReport, error) {
	return nil, errors.New("not implemented")
}
func (e *execAPITestEngine) RemoveNamedVolume(_ context.Context, _ string) error { return nil }
func (e *execAPITestEngine) InspectContainerRaw(_ string) (map[string]any, error) {
	return nil, errors.New("not implemented")
}
func (e *execAPITestEngine) SweepOrphanExecSessions(_ string, _ map[string]struct{}) error {
	e.sweepCalls.Add(1)
	return nil
}

func setupExecAPITest(t *testing.T, eng *execAPITestEngine) (*EdgeletAPIHandler, string) {
	t.Helper()
	db := store.GetInstance()
	_ = db.Close()
	if err := db.Open(t.TempDir()); err != nil {
		t.Fatalf("failed to open store db: %v", err)
	}
	if err := store.GetInstance().UpsertLocalWorkload(&models.LocalDeployedMicroservice{
		LocalUUID:    "ms-1",
		ManifestYAML: "apiVersion: edgelet.iofog.org/v1\nkind: Microservice\nmetadata:\n  name: test\nspec:\n  image: busybox\n",
		DesiredState: "running",
	}); err != nil {
		t.Fatalf("upsert local workload: %v", err)
	}
	eng.container = &engine.Container{ID: "0123456789abcdef0123456789abcdef"}
	processmanager.ConfigureEngineForTest(eng)
	processmanager.ResetExecRegistryForTest()
	t.Cleanup(func() {
		processmanager.ResetExecStartGateTimeoutForTest()
		processmanager.ResetExecRegistryForTest()
		processmanager.ResetProcessManagerEngineForTest()
	})
	return NewEdgeletAPIHandler(), "ms-1"
}

func decodeExecSuccessData(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var envelope apiSuccessEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode envelope: %v body=%s", err, rec.Body.String())
	}
	if !envelope.Success {
		t.Fatalf("expected success envelope: %s", rec.Body.String())
	}
	data, ok := envelope.Data.(map[string]any)
	if !ok {
		t.Fatalf("expected map data, got %T", envelope.Data)
	}
	return data
}

func decodeErrorEnvelope(t *testing.T, rec *httptest.ResponseRecorder) apiErrorEnvelope {
	t.Helper()
	var envelope apiErrorEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode error envelope: %v body=%s", err, rec.Body.String())
	}
	return envelope
}

func interactiveExecCreateBody() string {
	b, err := json.Marshal(map[string]any{"command": containerexec.ShellCommandInteractive()})
	if err != nil {
		panic(err)
	}
	return string(b)
}

func TestHandleCreateExecSession_TwoConcurrentPOSTDistinctActive(t *testing.T) {
	eng := &execAPITestEngine{}
	handler, msSelector := setupExecAPITest(t, eng)

	create := func() map[string]any {
		req := httptest.NewRequest(http.MethodPost, "/v1/ms/"+msSelector+"/exec/sessions", bytes.NewBufferString(interactiveExecCreateBody()))
		rec := httptest.NewRecorder()
		handler.HandleMicroservices(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("create status=%d body=%s", rec.Code, rec.Body.String())
		}
		return decodeExecSuccessData(t, rec)
	}

	data1 := create()
	data2 := create()
	id1, ok := data1["sessionId"].(string)
	if !ok || id1 == "" {
		t.Fatalf("expected sessionId string in data1, got %v", data1["sessionId"])
	}
	id2, ok := data2["sessionId"].(string)
	if !ok || id2 == "" {
		t.Fatalf("expected sessionId string in data2, got %v", data2["sessionId"])
	}
	if id1 == id2 {
		t.Fatalf("expected distinct session ids, got %q and %q", id1, id2)
	}
	if data1["status"] != "ACTIVE" || data2["status"] != "ACTIVE" {
		t.Fatalf("expected ACTIVE status, got %v and %v", data1["status"], data2["status"])
	}
}

func TestHandleCreateExecSession_ExecStartTimeout(t *testing.T) {
	eng := &execAPITestEngine{neverReady: true}
	handler, msSelector := setupExecAPITest(t, eng)
	processmanager.SetExecStartGateTimeoutForTest(200 * time.Millisecond)

	req := httptest.NewRequest(http.MethodPost, "/v1/ms/"+msSelector+"/exec/sessions", bytes.NewBufferString(`{}`))
	rec := httptest.NewRecorder()
	handler.HandleMicroservices(rec, req)
	if rec.Code != http.StatusGatewayTimeout {
		t.Fatalf("expected 504, got %d body=%s", rec.Code, rec.Body.String())
	}
	envelope := decodeErrorEnvelope(t, rec)
	if envelope.Error.Code != ErrCodeExecStartTimeout {
		t.Fatalf("expected %s, got %s", ErrCodeExecStartTimeout, envelope.Error.Code)
	}
}

func TestHandleCreateExecSession_OneShotReturnsPending(t *testing.T) {
	eng := &execAPITestEngine{neverReady: true}
	handler, msSelector := setupExecAPITest(t, eng)
	processmanager.SetExecStartGateTimeoutForTest(200 * time.Millisecond)

	req := httptest.NewRequest(http.MethodPost, "/v1/ms/"+msSelector+"/exec/sessions", bytes.NewBufferString(`{"command":["nslookup","edgelet.local-dns-b"]}`))
	rec := httptest.NewRecorder()
	handler.HandleMicroservices(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for one-shot, got %d body=%s", rec.Code, rec.Body.String())
	}
	data := decodeExecSuccessData(t, rec)
	if data["status"] != "PENDING" {
		t.Fatalf("expected PENDING for one-shot, got %v", data["status"])
	}
}

func TestHandleAttachExecSessionWS_UnknownSessionReturns404(t *testing.T) {
	eng := &execAPITestEngine{}
	handler, msSelector := setupExecAPITest(t, eng)

	req := httptest.NewRequest(http.MethodGet, "/v1/ms/"+msSelector+"/exec/sessions/missing-id:attach", nil)
	rec := httptest.NewRecorder()
	handler.HandleMicroservices(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandleAttachExecSessionWS_LocalDetachDoesNotStopControllerSession(t *testing.T) {
	eng := &execAPITestEngine{}
	handler, msSelector := setupExecAPITest(t, eng)
	pm := processmanager.GetInstance()
	containerID := eng.container.ID
	controllerRuntimeID := containerID[:12] + "-exec-ctrl-ses"
	_ = processmanager.RegisterExecSessionForTest(&processmanager.ExecSessionRecord{
		SessionID:     "ctrl-session",
		MSUUID:        "ms-1",
		ContainerID:   containerID,
		RuntimeExecID: controllerRuntimeID,
		Owner:         processmanager.ExecOwnerController,
		Started:       true,
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/ms/"+msSelector+"/exec/sessions", bytes.NewBufferString(`{"command":["sh"]}`))
	rec := httptest.NewRecorder()
	handler.HandleMicroservices(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("create local session: status=%d body=%s", rec.Code, rec.Body.String())
	}
	data := decodeExecSuccessData(t, rec)
	localSessionID, ok := data["sessionId"].(string)
	if !ok || localSessionID == "" {
		t.Fatalf("expected sessionId in create response, got %v", data["sessionId"])
	}
	localRec, ok := pm.GetSession(localSessionID)
	if !ok {
		t.Fatal("expected local session in registry")
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.handleAttachExecSessionWS(w, r, msSelector, localSessionID)
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial attach ws: %v", err)
	}
	_ = conn.Close()
	time.Sleep(100 * time.Millisecond)

	eng.stopMu.Lock()
	stops := append([]string(nil), eng.stopCalls...)
	eng.stopMu.Unlock()

	for _, stopped := range stops {
		if stopped == controllerRuntimeID {
			t.Fatalf("local detach stopped controller runtime exec %q", stopped)
		}
	}
	if _, ok := pm.GetSession("ctrl-session"); !ok {
		t.Fatal("controller session should remain in registry after local detach")
	}
	if _, ok := pm.GetSession(localSessionID); ok {
		t.Fatal("local session should be removed from registry after detach")
	}
	if localRec.RuntimeExecID != "" {
		foundLocalStop := false
		for _, stopped := range stops {
			if stopped == localRec.RuntimeExecID {
				foundLocalStop = true
				break
			}
		}
		if !foundLocalStop {
			t.Fatalf("expected local runtime exec %q to be stopped, stops=%v", localRec.RuntimeExecID, stops)
		}
	}
}

func TestHandleAttachExecSessionWS_ControllerSessionNotAttachable(t *testing.T) {
	eng := &execAPITestEngine{}
	handler, msSelector := setupExecAPITest(t, eng)
	_ = processmanager.RegisterExecSessionForTest(&processmanager.ExecSessionRecord{
		SessionID:     "ctrl-only",
		MSUUID:        "ms-1",
		ContainerID:   eng.container.ID,
		RuntimeExecID: "0123456789ab-exec-ctrl-on",
		Owner:         processmanager.ExecOwnerController,
		Started:       true,
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/ms/"+msSelector+"/exec/sessions/ctrl-only:attach", nil)
	rec := httptest.NewRecorder()
	handler.HandleMicroservices(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for controller-owned session attach, got %d body=%s", rec.Code, rec.Body.String())
	}
}
