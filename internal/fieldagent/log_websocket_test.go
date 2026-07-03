package fieldagent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eclipse-iofog/edgelet/internal/auth"
	"github.com/eclipse-iofog/edgelet/internal/config"
	"github.com/gorilla/websocket"
)

func newTestLogHandler(wsURL, sessionID string) *LogSessionWebSocketHandler {
	ctx, cancel := context.WithCancel(context.Background())
	h := &LogSessionWebSocketHandler{
		controllerWsURL: wsURL,
		sessionID:       sessionID,
		outputBuffer:    make(chan logFrame, logMaxBufferedFrames),
		ctx:             ctx,
		cancel:          cancel,
		config:          config.GetInstance(),
		jwtManager:      auth.GetJWTManager(),
	}
	h.state.Store(LogStateDisconnected)
	h.isConnected.Store(false)
	h.isActive.Store(false)
	return h
}

func TestLogHandler_ResetClearsStalePendingState(t *testing.T) {
	h := newTestLogHandler("ws://unused", "log-session-1")
	h.isConnected.Store(false)
	h.state.Store(LogStatePending)

	h.Reset()

	if got := h.GetConnectionState(); got != LogStateDisconnected {
		t.Fatalf("state = %v, want LogStateDisconnected", got)
	}
	if h.IsConnected() {
		t.Fatal("expected not connected after reset")
	}
}

func TestLogHandler_DisconnectClearsStaleActiveState(t *testing.T) {
	h := newTestLogHandler("ws://unused", "log-session-2")
	h.isConnected.Store(false)
	h.state.Store(LogStateActive)

	h.Disconnect()

	if got := h.GetConnectionState(); got != LogStateDisconnected {
		t.Fatalf("state = %v, want LogStateDisconnected", got)
	}
}

func TestLogHandler_readWaitDuration(t *testing.T) {
	h := newTestLogHandler("ws://unused", "log-session-duration")
	if got := h.readWaitDuration(); got != logPendingReadTimeout {
		t.Fatalf("pending readWaitDuration = %v, want %v", got, logPendingReadTimeout)
	}
	h.isActive.Store(true)
	if got := h.readWaitDuration(); got != 0 {
		t.Fatalf("active readWaitDuration = %v, want 0 (disabled)", got)
	}
}

func TestLogHandler_activeStreamSurvivesIdleWithControllerPing(t *testing.T) {
	setupExecTestJWT(t)

	upgrader := websocketUpgrader()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		done := make(chan struct{})
		defer close(done)

		go func() {
			for {
				select {
				case <-done:
					return
				case <-ticker.C:
					_ = conn.WriteControl(websocket.PingMessage, []byte("keepalive"), time.Now().Add(time.Second))
				}
			}
		}()

		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	t.Cleanup(srv.Close)

	sessionID := "log-idle-active"
	msUUID := "ms-idle-active"
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/agent/logs/microservice/" + msUUID + "/" + sessionID
	h := newTestLogHandler(wsURL, sessionID)
	if err := h.Connect(); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	startPayload, err := json.Marshal(map[string]any{
		"tailConfig": map[string]any{"tail": float64(100), "follow": true},
	})
	if err != nil {
		t.Fatalf("marshal LOG_START payload: %v", err)
	}
	h.handleLogStart(startPayload, sessionID)

	time.Sleep(350 * time.Millisecond)

	if !h.IsConnected() {
		t.Fatal("expected active log WebSocket to stay connected during idle Controller ping traffic")
	}
	if !h.IsActive() {
		t.Fatal("expected handler to remain active")
	}

	h.Reset()
}

func TestLogHandler_respondsToControllerPingWithPong(t *testing.T) {
	setupExecTestJWT(t)

	var pongReceived atomic.Bool
	upgrader := websocketUpgrader()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		conn.SetPongHandler(func(string) error {
			pongReceived.Store(true)
			return nil
		})

		go func() {
			for {
				if _, _, err := conn.ReadMessage(); err != nil {
					return
				}
			}
		}()

		_ = conn.WriteControl(websocket.PingMessage, []byte("ping"), time.Now().Add(time.Second))

		deadline := time.Now().Add(2 * time.Second)
		for !pongReceived.Load() {
			if time.Now().After(deadline) {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
	}))
	t.Cleanup(srv.Close)

	sessionID := "log-pong"
	msUUID := "ms-pong"
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/agent/logs/microservice/" + msUUID + "/" + sessionID
	h := newTestLogHandler(wsURL, sessionID)
	if err := h.Connect(); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for !pongReceived.Load() {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for pong response to Controller ping")
		}
		time.Sleep(20 * time.Millisecond)
	}

	h.Reset()
}

func TestLogHandler_DisconnectSendsGracefulClose(t *testing.T) {
	setupExecTestJWT(t)

	var closeCode atomic.Int32
	upgrader := websocketUpgrader()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		for {
			messageType, data, err := conn.ReadMessage()
			if err != nil {
				var closeErr *websocket.CloseError
				if errors.As(err, &closeErr) {
					closeCode.Store(int32(closeErr.Code))
				}
				return
			}
			if messageType == websocket.CloseMessage {
				closeCode.Store(int32(websocket.CloseNormalClosure))
				return
			}
			_ = data
		}
	}))
	t.Cleanup(srv.Close)

	sessionID := "log-graceful"
	msUUID := "ms-graceful"
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/agent/logs/microservice/" + msUUID + "/" + sessionID
	h := newTestLogHandler(wsURL, sessionID)
	if err := h.Connect(); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	h.Disconnect()

	deadline := time.Now().Add(2 * time.Second)
	for closeCode.Load() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for graceful close frame")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if code := closeCode.Load(); code != websocket.CloseNormalClosure {
		t.Fatalf("close code = %d, want %d", code, websocket.CloseNormalClosure)
	}
}

func TestLogHandler_handleLogStartPayload(t *testing.T) {
	h := newTestLogHandler("ws://unused", "log-start")
	h.state.Store(LogStatePending)

	payload := bytes.NewBuffer(nil)
	if err := json.NewEncoder(payload).Encode(map[string]any{
		"tailConfig": map[string]any{"tail": float64(50)},
	}); err != nil {
		t.Fatalf("encode payload: %v", err)
	}

	h.handleLogStart(payload.Bytes(), "log-start")
	if !h.IsActive() {
		t.Fatal("expected active after LOG_START")
	}
	if got := h.GetConnectionState(); got != LogStateActive {
		t.Fatalf("state = %v, want LogStateActive", got)
	}
}
