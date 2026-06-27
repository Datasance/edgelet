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

func TestExecSessionManager_FetchExecSessions_Parse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.HasSuffix(r.URL.Path, "/agent/exec/sessions") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"execSessions": []any{
				map[string]any{
					"sessionId":        "sess-pending",
					"microserviceUuid": "ms-1",
					"status":           "PENDING",
					"agentConnected":   false,
				},
				map[string]any{
					"sessionId":        "sess-active",
					"microserviceUuid": "ms-2",
					"status":           "ACTIVE",
					"agentConnected":   true,
				},
			},
		})
	}))
	t.Cleanup(srv.Close)

	cfg := config.GetInstance()
	cfg.ControllerURL = srv.URL + "/api/v3/"

	fa := GetInstance()
	fa.apiClient = &APIClient{
		baseURL:    cfg.ControllerURL,
		httpClient: srv.Client(),
		jwtManager: auth.GetJWTManager(),
	}
	fa.state.SetControllerStatus(models.ControllerStatusOK)
	fa.state.SetControllerVerified(true)

	esm := GetExecSessionManager()
	sessions, err := esm.FetchExecSessions(context.Background())
	if err != nil {
		t.Fatalf("FetchExecSessions: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("len(sessions) = %d, want 2", len(sessions))
	}
	if sessions[0].SessionID != "sess-pending" || sessions[0].Status != "PENDING" {
		t.Fatalf("pending session = %+v", sessions[0])
	}
	if sessions[1].SessionID != "sess-active" || sessions[1].Status != "ACTIVE" || !sessions[1].AgentConnected {
		t.Fatalf("active session = %+v", sessions[1])
	}
}

func TestExecSessionManager_HandleExecSessions_Detach(t *testing.T) {
	esm := GetExecSessionManager()
	sessionID := "sess-detach"
	msUUID := "ms-detach"

	esm.mu.Lock()
	esm.activeSessions[sessionID] = &ExecSessionInfo{
		Session: &ExecSession{
			SessionID:        sessionID,
			MicroserviceUUID: msUUID,
			Status:           "ACTIVE",
		},
	}
	handler := newTestExecHandler("ws://unused", sessionID, msUUID)
	handler.state.Store(StatePending)
	esm.webSocketHandlers[sessionID] = handler
	esm.mu.Unlock()

	esm.HandleExecSessions([]*ExecSession{})

	esm.mu.RLock()
	_, exists := esm.activeSessions[sessionID]
	_, handlerExists := esm.webSocketHandlers[sessionID]
	esm.mu.RUnlock()

	if exists {
		t.Fatal("expected session removed after detach reconcile")
	}
	if handlerExists {
		t.Fatal("expected websocket handler removed after detach reconcile")
	}
	if got := handler.GetConnectionState(); got != StateDisconnected {
		t.Fatalf("handler state = %v, want StateDisconnected", got)
	}
}

func TestExecSessionManager_HandleExecSessions_Reconnect(t *testing.T) {
	setupExecTestJWT(t)

	var upgrades atomic.Int32
	upgrader := websocketUpgrader()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrades.Add(1)
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		_, _, _ = conn.ReadMessage()
	}))
	t.Cleanup(srv.Close)

	sessionID := "sess-reconnect"
	msUUID := "ms-reconnect"
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/agent/exec/microservice/" + msUUID + "/" + sessionID

	esm := GetExecSessionManager()
	handler := newTestExecHandler(wsURL, sessionID, msUUID)
	esm.mu.Lock()
	esm.activeSessions[sessionID] = &ExecSessionInfo{
		Session: &ExecSession{
			SessionID:        sessionID,
			MicroserviceUUID: msUUID,
			Status:           "ACTIVE",
		},
		Callback: NewExecSessionCallback(msUUID, sessionID),
	}
	esm.webSocketHandlers[sessionID] = handler
	handler.SetExecSessionManager(esm)
	esm.mu.Unlock()

	esm.HandleExecSessions([]*ExecSession{{
		SessionID:        sessionID,
		MicroserviceUUID: msUUID,
		Status:           "ACTIVE",
	}})

	if upgrades.Load() != 1 {
		t.Fatalf("expected 1 reconnect upgrade, got %d", upgrades.Load())
	}
	if !handler.IsConnected() {
		t.Fatal("expected handler connected after reconnect reconcile")
	}

	esm.StopExecSession(sessionID)
	handler.Reset()
}

func TestExecSessionManager_StopExecSessionDoesNotBlockManagerLock(t *testing.T) {
	esm := GetExecSessionManager()
	sessionID := "sess-stop-lock"
	handler := newTestExecHandler("ws://unused", sessionID, "ms-1")
	esm.mu.Lock()
	esm.activeSessions[sessionID] = &ExecSessionInfo{
		Session:  &ExecSession{SessionID: sessionID, MicroserviceUUID: "ms-1"},
		Callback: NewExecSessionCallback("ms-1", sessionID),
	}
	esm.webSocketHandlers[sessionID] = handler
	esm.mu.Unlock()

	stopDone := make(chan struct{})
	go func() {
		esm.StopExecSession(sessionID)
		close(stopDone)
	}()

	acquired := make(chan struct{})
	go func() {
		esm.mu.RLock()
		_ = len(esm.activeSessions)
		esm.mu.RUnlock()
		close(acquired)
	}()

	select {
	case <-acquired:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for esm.mu RLock during StopExecSession")
	}
	select {
	case <-stopDone:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for StopExecSession")
	}
}

func TestExecHandler_CloseMessageStopIsAsync(t *testing.T) {
	esm := GetExecSessionManager()
	sessionID := "sess-async-close"
	handler := newTestExecHandler("ws://unused", sessionID, "ms-1")
	handler.SetExecSessionManager(esm)
	esm.mu.Lock()
	esm.activeSessions[sessionID] = &ExecSessionInfo{
		Session:  &ExecSession{SessionID: sessionID, MicroserviceUUID: "ms-1"},
		Callback: NewExecSessionCallback("ms-1", sessionID),
	}
	esm.webSocketHandlers[sessionID] = handler
	esm.mu.Unlock()

	handler.handleSessionClose()

	acquired := make(chan struct{})
	go func() {
		esm.mu.RLock()
		_ = len(esm.activeSessions)
		esm.mu.RUnlock()
		close(acquired)
	}()
	select {
	case <-acquired:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("handleSessionClose blocked esm.mu")
	}

	stopDone := make(chan struct{})
	go func() {
		esm.StopExecSession(sessionID)
		close(stopDone)
	}()
	select {
	case <-stopDone:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for cleanup StopExecSession")
	}
}

func TestExecSessionWebSocketURL_ContainsSessionID(t *testing.T) {
	cfg := config.GetInstance()
	cfg.ControllerURL = "http://controller.example.com:54421/api/v3/"

	sessionID := "abc-session-id"
	msUUID := "ms-url-test"
	handler := GetExecSessionWebSocketHandler(sessionID, msUUID)
	if handler == nil {
		t.Fatal("handler is nil")
	}
	want := "/agent/exec/microservice/" + msUUID + "/" + sessionID
	if !strings.Contains(handler.controllerWsURL, want) {
		t.Fatalf("controllerWsURL = %q, want substring %q", handler.controllerWsURL, want)
	}
}
