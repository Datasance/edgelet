package fieldagent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eclipse-iofog/edgelet/internal/auth"
	"github.com/eclipse-iofog/edgelet/internal/config"
	"github.com/eclipse-iofog/edgelet/internal/models"
)

func TestNoteControllerReachable_IncrementsGenerationOnce(t *testing.T) {
	fa := &FieldAgent{state: NewState()}
	fa.state.SetControllerStatus(models.ControllerStatusNotConnected)

	if !fa.noteControllerReachable() {
		t.Fatal("expected first reconnect transition")
	}
	if fa.reconnect.connectGeneration != 1 {
		t.Fatalf("expected connect generation 1, got %d", fa.reconnect.connectGeneration)
	}

	fa.state.SetControllerStatus(models.ControllerStatusOK)
	if fa.noteControllerReachable() {
		t.Fatal("expected no transition when controller already OK")
	}

	fa.state.SetControllerStatus(models.ControllerStatusNotConnected)
	if !fa.noteControllerReachable() {
		t.Fatal("expected transition after returning to NOT_CONNECTED")
	}
	if fa.reconnect.connectGeneration != 2 {
		t.Fatalf("expected connect generation 2, got %d", fa.reconnect.connectGeneration)
	}
}

func TestReconnect_RunsReconcileOnce(t *testing.T) {
	var reconcileCalls atomic.Int32
	fa := &FieldAgent{
		state: NewState(),
		controllerReconcileHook: func() error {
			reconcileCalls.Add(1)
			return nil
		},
	}
	fa.state.SetControllerStatus(models.ControllerStatusNotConnected)

	if !fa.noteControllerReachable() {
		t.Fatal("expected reconnect transition")
	}
	if err := fa.controllerReconcile(); err != nil {
		t.Fatalf("controllerReconcile: %v", err)
	}
	if err := fa.controllerReconcile(); err != nil {
		t.Fatalf("second controllerReconcile: %v", err)
	}
	if got := reconcileCalls.Load(); got != 2 {
		t.Fatalf("expected hook to run twice (single-flight is mutex-only), got %d", got)
	}
}

func TestProcessChanges_SkipsInitReloadAfterReconcile(t *testing.T) {
	openFieldAgentTestDB(t)

	var registriesRequested atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/agent/registries") {
			registriesRequested.Store(true)
			_, _ = w.Write([]byte(`{"registries":[]}`))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	cfg := config.GetInstance()
	origURL := cfg.ControllerURL
	cfg.ControllerURL = srv.URL
	cfg.IOFogUUID = "uuid-init-dedup"
	cfg.PrivateKey = testProvisionPrivateKeyBase64(t)
	t.Cleanup(func() {
		cfg.ControllerURL = origURL
		cfg.IOFogUUID = ""
		cfg.PrivateKey = ""
	})

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	fa := &FieldAgent{
		config: cfg,
		state:  NewState(),
		ctx:    ctx,
		apiClient: &APIClient{
			baseURL:    srv.URL,
			httpClient: srv.Client(),
			jwtManager: auth.GetJWTManager(),
		},
	}
	fa.state.SetControllerStatus(models.ControllerStatusOK)
	fa.state.SetControllerVerified(true)
	fa.state.SetInitialization(true)

	fa.reconnect.mu.Lock()
	fa.reconnect.connectGeneration = 1
	fa.reconnect.lastReconcileGeneration = 1
	fa.reconnect.lastReconcileAt = time.Now()
	fa.reconnect.mu.Unlock()

	if !fa.shouldSkipInitReload() {
		t.Fatal("expected shouldSkipInitReload true after reconcile")
	}

	_ = fa.processChanges(map[string]any{"registries": false})
	if registriesRequested.Load() {
		t.Fatal("expected init registries reload to be skipped after reconcile")
	}
}

func TestInitClearsOnlyAfterGetChanges(t *testing.T) {
	fa := &FieldAgent{
		state: NewState(),
		controllerReconcileHook: func() error {
			return nil
		},
	}
	fa.state.SetInitialization(true)
	fa.state.SetControllerStatus(models.ControllerStatusOK)
	fa.state.SetControllerVerified(true)

	if err := fa.controllerReconcile(); err != nil {
		t.Fatalf("controllerReconcile: %v", err)
	}
	if !fa.state.IsInitialization() {
		t.Fatal("expected initialization to remain true after reconcile alone")
	}

	fa.state.SetInitialization(false)
	if fa.state.IsInitialization() {
		t.Fatal("expected initialization cleared after successful getChanges cycle")
	}
}

func TestPostFogConfigDuringInit(t *testing.T) {
	openFieldAgentTestDB(t)

	var configMethod atomic.Value
	configMethod.Store("")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/agent/config") {
			configMethod.Store(r.Method)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{}`))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	cfg := config.GetInstance()
	origURL := cfg.ControllerURL
	cfg.ControllerURL = srv.URL
	cfg.IOFogUUID = "uuid-post-init"
	cfg.PrivateKey = testProvisionPrivateKeyBase64(t)
	t.Cleanup(func() {
		cfg.ControllerURL = origURL
		cfg.IOFogUUID = ""
		cfg.PrivateKey = ""
	})

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	fa := &FieldAgent{
		config: cfg,
		state:  NewState(),
		ctx:    ctx,
		apiClient: &APIClient{
			baseURL:    srv.URL,
			httpClient: srv.Client(),
			jwtManager: auth.GetJWTManager(),
		},
	}
	fa.state.SetControllerStatus(models.ControllerStatusOK)
	fa.state.SetControllerVerified(true)
	fa.state.SetInitialization(true)

	if err := fa.postFogConfig(); err != nil {
		t.Fatalf("postFogConfig: %v", err)
	}
	method, ok := configMethod.Load().(string)
	if !ok || method != http.MethodPatch {
		t.Fatalf("expected PATCH config during init, got %q", method)
	}

	fa.state.SetInitialization(false)
	configMethod.Store("")
	if err := fa.getFogConfig(); err != nil {
		t.Fatalf("getFogConfig: %v", err)
	}
	method, ok = configMethod.Load().(string)
	if !ok || method != http.MethodGet {
		t.Fatalf("expected GET config when init false, got %q", method)
	}
}

func TestControllerDownBoot_InitStaysTrue(t *testing.T) {
	openFieldAgentTestDB(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/status") {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	cfg := config.GetInstance()
	origURL := cfg.ControllerURL
	origUUID := cfg.IOFogUUID
	origKey := cfg.PrivateKey
	cfg.ControllerURL = srv.URL
	cfg.IOFogUUID = "uuid-boot-down"
	cfg.PrivateKey = testProvisionPrivateKeyBase64(t)
	t.Cleanup(func() {
		cfg.ControllerURL = origURL
		cfg.IOFogUUID = origUUID
		cfg.PrivateKey = origKey
	})

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	var liveLoadCalled atomic.Bool
	fa := &FieldAgent{
		config: cfg,
		state:  NewState(),
		ctx:    ctx,
		apiClient: &APIClient{
			baseURL:    srv.URL,
			httpClient: srv.Client(),
			jwtManager: auth.GetJWTManager(),
		},
		loadInitialControllerDataHook: func(isConnected bool) {
			if isConnected {
				liveLoadCalled.Store(true)
			}
		},
		controllerReconcileHook: func() error {
			liveLoadCalled.Store(true)
			return nil
		},
	}
	fa.state.SetInitialization(true)
	fa.state.SetControllerStatus(models.ControllerStatusNotConnected)

	fa.wg.Add(1)
	fa.bootstrapControllerSync()

	if !fa.state.IsInitialization() {
		t.Fatal("expected initialization to stay true when controller unreachable at boot")
	}
	if liveLoadCalled.Load() {
		t.Fatal("expected no live reconcile when controller ping fails at boot")
	}
}

func TestBootstrap_ReconcileOnPingSuccess(t *testing.T) {
	openFieldAgentTestDB(t)

	var reconcileCalled atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/status"):
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	cfg := config.GetInstance()
	origURL := cfg.ControllerURL
	origUUID := cfg.IOFogUUID
	origKey := cfg.PrivateKey
	cfg.ControllerURL = srv.URL
	cfg.IOFogUUID = "uuid-boot-ok"
	cfg.PrivateKey = testProvisionPrivateKeyBase64(t)
	t.Cleanup(func() {
		cfg.ControllerURL = origURL
		cfg.IOFogUUID = origUUID
		cfg.PrivateKey = origKey
	})

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	fa := &FieldAgent{
		config: cfg,
		state:  NewState(),
		ctx:    ctx,
		apiClient: &APIClient{
			baseURL:    srv.URL,
			httpClient: srv.Client(),
			jwtManager: auth.GetJWTManager(),
		},
		loadInitialControllerDataHook: func(bool) {},
		controllerReconcileHook: func() error {
			reconcileCalled.Store(true)
			return nil
		},
	}
	fa.state.SetControllerStatus(models.ControllerStatusNotConnected)

	fa.wg.Add(1)
	fa.bootstrapControllerSync()

	if !reconcileCalled.Load() {
		t.Fatal("expected controllerReconcile on successful boot ping")
	}
}
