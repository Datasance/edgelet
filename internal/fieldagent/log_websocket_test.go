package fieldagent

import (
	"context"
	"testing"

	"github.com/eclipse-iofog/edgelet/internal/auth"
	"github.com/eclipse-iofog/edgelet/internal/config"
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
