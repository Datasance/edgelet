package fieldagent

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eclipse-iofog/edgelet/internal/auth"
	"github.com/eclipse-iofog/edgelet/internal/config"
	"github.com/eclipse-iofog/edgelet/internal/store"
	"github.com/gorilla/websocket"
	"github.com/vmihailenco/msgpack/v5"
)

func TestConvertToWebSocketURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "HTTP URL",
			input:    "http://controller.example.com:54421/api/v2/",
			expected: "ws://controller.example.com:54421/api/v2/",
		},
		{
			name:     "HTTPS URL",
			input:    "https://controller.example.com:54421/api/v2/",
			expected: "wss://controller.example.com:54421/api/v2/",
		},
		{
			name:     "Already WebSocket URL",
			input:    "ws://controller.example.com:54421/api/v2/",
			expected: "ws://controller.example.com:54421/api/v2/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertToWebSocketURL(tt.input)
			if result != tt.expected {
				t.Errorf("convertToWebSocketURL(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func setupExecTestJWT(t *testing.T) {
	t.Helper()

	db := store.GetInstance()
	if err := db.Open(t.TempDir()); err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	_, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	seed := privateKey.Seed()
	jwk := map[string]any{
		"kty": "OKP",
		"crv": "Ed25519",
		"d":   base64.RawURLEncoding.EncodeToString(seed),
		"x":   base64.RawURLEncoding.EncodeToString(privateKey.Public().(ed25519.PublicKey)),
	}
	jwkJSON, err := json.Marshal(jwk)
	if err != nil {
		t.Fatalf("marshal jwk: %v", err)
	}

	cfg := config.GetInstance()
	cfg.PrivateKey = base64.StdEncoding.EncodeToString(jwkJSON)
	cfg.IOFogUUID = "test-agent-uuid"
	cfg.SecureMode = false

	auth.GetJWTManager().Reset()
}

func newTestExecHandler(wsURL, microserviceUUID string) *ExecSessionWebSocketHandler {
	ctx, cancel := context.WithCancel(context.Background())
	h := &ExecSessionWebSocketHandler{
		controllerWsURL:  wsURL,
		microserviceUUID: microserviceUUID,
		outputBuffer:     make(chan []byte, maxBufferedFrames),
		ctx:              ctx,
		cancel:           cancel,
		config:           config.GetInstance(),
		jwtManager:       auth.GetJWTManager(),
	}
	h.state.Store(StateDisconnected)
	h.isConnected.Store(false)
	h.isActive.Store(false)
	return h
}

func TestExecHandler_ResetClearsStalePendingState(t *testing.T) {
	h := newTestExecHandler("ws://unused", "ms-stale")
	h.isConnected.Store(false)
	h.state.Store(StatePending)

	h.Reset()

	if got := h.GetConnectionState(); got != StateDisconnected {
		t.Fatalf("state = %v, want StateDisconnected", got)
	}
	if h.IsConnected() {
		t.Fatal("expected not connected after reset")
	}
	if h.IsActive() {
		t.Fatal("expected not active after reset")
	}
}

func TestExecHandler_DisconnectClearsStalePendingState(t *testing.T) {
	h := newTestExecHandler("ws://unused", "ms-disconnect")
	h.isConnected.Store(false)
	h.state.Store(StatePending)

	h.Disconnect()

	if got := h.GetConnectionState(); got != StateDisconnected {
		t.Fatalf("state = %v, want StateDisconnected", got)
	}
}

func TestExecHandler_ConnectAfterStalePendingState(t *testing.T) {
	setupExecTestJWT(t)

	var upgrades atomic.Int32
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrades.Add(1)
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		_, _, _ = conn.ReadMessage()
		_ = conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	}))
	t.Cleanup(srv.Close)

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/agent/exec/ms-connect"
	h := newTestExecHandler(wsURL, "ms-connect")
	h.isConnected.Store(false)
	h.state.Store(StatePending)

	if err := h.Connect(); err != nil {
		t.Fatalf("Connect after stale pending state: %v", err)
	}
	if upgrades.Load() != 1 {
		t.Fatalf("expected 1 websocket upgrade, got %d", upgrades.Load())
	}
	if h.GetConnectionState() != StatePending {
		t.Fatalf("state = %v, want StatePending after connect", h.GetConnectionState())
	}

	h.Reset()

	if err := h.Connect(); err != nil {
		t.Fatalf("second Connect after reset: %v", err)
	}
	if upgrades.Load() != 2 {
		t.Fatalf("expected 2 websocket upgrades after retry, got %d", upgrades.Load())
	}

	h.Reset()
}

func TestExecHandler_ConnectSendsInitMessage(t *testing.T) {
	setupExecTestJWT(t)

	initCh := make(chan map[string]string, 1)
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		_, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		dec := msgpack.NewDecoder(bytes.NewReader(data))
		n, err := dec.DecodeMapLen()
		if err != nil {
			return
		}
		msg := make(map[string]string)
		for i := 0; i < n; i++ {
			key, err := dec.DecodeString()
			if err != nil {
				return
			}
			val, err := dec.DecodeString()
			if err != nil {
				return
			}
			msg[key] = val
		}
		initCh <- msg
		_ = conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	}))
	t.Cleanup(srv.Close)

	msUUID := "ms-init"
	execID := "exec-test-id"
	fa := &FieldAgent{
		activeExecSessions: map[string]string{msUUID: execID},
		execCallbacks:      map[string]*ExecSessionCallback{},
		activeWebSockets:   map[string]*ExecSessionWebSocketHandler{},
	}
	// Override singleton for test
	prev := instance
	instance = fa
	t.Cleanup(func() { instance = prev })

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/agent/exec/" + msUUID
	h := newTestExecHandler(wsURL, msUUID)

	if err := h.Connect(); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	select {
	case msg := <-initCh:
		if msg["execId"] != execID {
			t.Fatalf("init execId = %q, want %q", msg["execId"], execID)
		}
		if msg["microserviceUuid"] != msUUID {
			t.Fatalf("init microserviceUuid = %q, want %q", msg["microserviceUuid"], msUUID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for init message")
	}

	h.Reset()
}

func TestExecHandler_ResetIsolationBetweenMicroservices(t *testing.T) {
	hA := newTestExecHandler("ws://unused/a", "ms-a")
	hB := newTestExecHandler("ws://unused/b", "ms-b")

	hA.isConnected.Store(false)
	hA.state.Store(StatePending)
	hA.Reset()

	if hB.GetConnectionState() != StateDisconnected {
		t.Fatalf("ms-b state changed unexpectedly: %v", hB.GetConnectionState())
	}

	hB.state.Store(StateActive)
	hB.isConnected.Store(true)
}
