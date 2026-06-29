package fieldagent

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/eclipse-iofog/edgelet/internal/auth"
	"github.com/eclipse-iofog/edgelet/internal/config"
	"github.com/eclipse-iofog/edgelet/internal/models"
	"github.com/eclipse-iofog/edgelet/internal/processmanager"
	"github.com/eclipse-iofog/edgelet/internal/store"
	"github.com/eclipse-iofog/edgelet/internal/workloadmeta"
	"github.com/eclipse-iofog/edgelet/pkg/engine"
)

func setupControllerRegisterIntegrationTest(t *testing.T, uuid string, registerHandler http.HandlerFunc) (*FieldAgent, *atomic.Int32) {
	t.Helper()
	openFieldAgentTestDB(t)

	var registerCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/agent/controller/register") {
			registerCalls.Add(1)
			if registerHandler != nil {
				registerHandler(w, r)
				return
			}
			_, _ = w.Write([]byte(`{"uuid":"` + uuid + `"}`))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	eng := &controllerRegisterTestEngine{
		containerID:      "cid-reg",
		microserviceUUID: uuid,
	}
	processmanager.ConfigureEngineForTest(eng)
	t.Cleanup(processmanager.ResetProcessManagerEngineForTest)

	if err := store.GetInstance().UpsertSystemControlPlane(minimalControlPlaneForReconcileTest(uuid)); err != nil {
		t.Fatalf("upsert control plane: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	cfg := config.GetInstance()
	origURL := cfg.ControllerURL
	cfg.ControllerURL = srv.URL
	t.Cleanup(func() { cfg.ControllerURL = origURL })

	fa := &FieldAgent{
		config:             cfg,
		state:              NewState(),
		ctx:                ctx,
		controllerRegister: newControllerRegisterState(),
		processManager:     processmanager.GetInstance(),
	}
	fa.state.SetControllerStatus(models.ControllerStatusOK)
	fa.state.SetControllerVerified(true)
	fa.apiClient = &APIClient{
		baseURL:    srv.URL,
		httpClient: srv.Client(),
		jwtManager: auth.GetJWTManager(),
	}

	return fa, &registerCalls
}

func TestSyncControllerRegister_SkipsWhenAlreadySucceeded(t *testing.T) {
	const uuid = "cp-reg-skip"
	fa, calls := setupControllerRegisterIntegrationTest(t, uuid, nil)
	fa.controllerRegister.markSucceeded(uuid)

	if fa.SyncControllerRegister(false) {
		t.Fatal("expected skip without force when already succeeded")
	}
	if got := calls.Load(); got != 0 {
		t.Fatalf("expected no register call, got %d", got)
	}
}

func TestSyncControllerRegister_ForceUpsertsWhenAlreadySucceeded(t *testing.T) {
	const uuid = "cp-reg-force"
	fa, calls := setupControllerRegisterIntegrationTest(t, uuid, nil)
	fa.controllerRegister.markSucceeded(uuid)

	if !fa.SyncControllerRegister(true) {
		t.Fatal("expected forced register to succeed")
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("expected one register upsert, got %d", got)
	}
}

type controllerRegisterTestEngine struct {
	containerID      string
	microserviceUUID string
}

func (e *controllerRegisterTestEngine) Init(engine.EngineConfig) error { return nil }
func (e *controllerRegisterTestEngine) Close() error                   { return nil }

func (e *controllerRegisterTestEngine) GetContainer(msUUID string) (*engine.Container, error) {
	if strings.TrimSpace(msUUID) == strings.TrimSpace(e.microserviceUUID) && e.containerID != "" {
		return &engine.Container{
			ID: e.containerID,
			Labels: map[string]string{
				workloadmeta.LabelMicroserviceUID: e.microserviceUUID,
			},
		}, nil
	}
	return nil, nil
}

func (e *controllerRegisterTestEngine) GetContainerByID(string) (*engine.Container, error) {
	return nil, nil
}
func (e *controllerRegisterTestEngine) GetContainerSandboxID(string) (string, error) { return "", nil }
func (e *controllerRegisterTestEngine) GetRunningContainers() ([]engine.Container, error) {
	return nil, nil
}
func (e *controllerRegisterTestEngine) GetAllContainers() ([]engine.Container, error) {
	return nil, nil
}
func (e *controllerRegisterTestEngine) CreateContainer(*models.Microservice, string) (string, error) {
	return "", nil
}
func (e *controllerRegisterTestEngine) StartContainer(string) error { return nil }
func (e *controllerRegisterTestEngine) StopContainer(string) error  { return nil }
func (e *controllerRegisterTestEngine) KillContainer(string) error  { return nil }
func (e *controllerRegisterTestEngine) RemoveContainer(string, bool) error {
	return nil
}
func (e *controllerRegisterTestEngine) PullImage(string, *models.Registry, *engine.PullImageOptions) error {
	return nil
}
func (e *controllerRegisterTestEngine) FindLocalImage(string, string, bool) (bool, error) {
	return true, nil
}
func (e *controllerRegisterTestEngine) RemoveImage(string) error { return nil }
func (e *controllerRegisterTestEngine) PruneImages() error       { return nil }
func (e *controllerRegisterTestEngine) ListImages(context.Context) ([]engine.ImageInfo, error) {
	return nil, nil
}
func (e *controllerRegisterTestEngine) LoadImageFromPath(context.Context, string) ([]engine.LoadedImage, error) {
	return nil, nil
}
func (e *controllerRegisterTestEngine) DeleteImage(context.Context, string) error { return nil }
func (e *controllerRegisterTestEngine) PruneDangling(context.Context) (*engine.ImagePruneReport, error) {
	return nil, nil
}
func (e *controllerRegisterTestEngine) PruneContainers(context.Context) (*engine.ContainerPruneReport, error) {
	return nil, nil
}
func (e *controllerRegisterTestEngine) PruneVolumes(context.Context) (*engine.VolumePruneReport, error) {
	return nil, nil
}
func (e *controllerRegisterTestEngine) RemoveNamedVolume(context.Context, string) error { return nil }
func (e *controllerRegisterTestEngine) GetContainerStatus(string, string) (*models.MicroserviceStatus, error) {
	return nil, nil
}
func (e *controllerRegisterTestEngine) GetContainerStats(string) (*engine.ContainerStats, error) {
	return nil, nil
}
func (e *controllerRegisterTestEngine) GetContainerIPAddress(string) (string, error) { return "", nil }
func (e *controllerRegisterTestEngine) GetContainerStartedAt(string) (int64, error)  { return 0, nil }
func (e *controllerRegisterTestEngine) InspectContainerRaw(string) (map[string]any, error) {
	return nil, nil
}
func (e *controllerRegisterTestEngine) TailContainerLogs(string, string, string, engine.LogTailHandler, *engine.TailConfig) error {
	return nil
}
func (e *controllerRegisterTestEngine) AreMicroserviceAndContainerEqual(string, *models.Microservice, *models.Registry) bool {
	return false
}
func (e *controllerRegisterTestEngine) EnsureNetwork(string) error { return nil }
func (e *controllerRegisterTestEngine) CreateExecSession(string, string, []string) (string, error) {
	return "", nil
}
func (e *controllerRegisterTestEngine) StartExecSession(string, io.Reader, io.Writer, io.Writer) error {
	return nil
}
func (e *controllerRegisterTestEngine) GetExecSessionStatus(string) (bool, error)  { return false, nil }
func (e *controllerRegisterTestEngine) GetExecSessionExitCode(string) (int, error) { return 0, nil }
func (e *controllerRegisterTestEngine) ResizeExecSession(string, uint32, uint32) error {
	return nil
}
func (e *controllerRegisterTestEngine) StopExecSession(string) error { return nil }
func (e *controllerRegisterTestEngine) GetContainerMicroserviceUUID(engine.Container) string {
	return e.microserviceUUID
}
func (e *controllerRegisterTestEngine) GetContainerName(engine.Container) string { return "controller" }

var _ engine.ContainerEngine = (*controllerRegisterTestEngine)(nil)
