package fieldagent

import (
	"context"
	"testing"
	"time"
)

func TestLogSessionManager_HandleWebSocketTransportCloseStopsTail(t *testing.T) {
	lsm := &LogSessionManager{
		activeSessions: make(map[string]*LogSessionInfo),
		tailCallbacks:  make(map[string]*logTailHandler),
	}

	sessionID := "log-session-stop"
	tailCtx, cancel := context.WithCancel(context.Background())

	info := &LogSessionInfo{
		Session:     &LogSession{SessionID: sessionID},
		IsStreaming: true,
		tailCancel:  cancel,
	}
	lsm.activeSessions[sessionID] = info

	handler := &logTailHandler{sessionID: sessionID, lsm: lsm}
	lsm.tailCallbacks[sessionID] = handler

	lsm.HandleWebSocketTransportClose(sessionID)

	if info.IsStreaming {
		t.Fatal("expected IsStreaming=false after transport close")
	}
	if !handler.stopped.Load() {
		t.Fatal("expected tail handler marked stopped")
	}

	select {
	case <-tailCtx.Done():
	case <-time.After(time.Second):
		t.Fatal("expected tail context canceled")
	}
}

func TestLogSessionManager_expireOldLogSessionsLocked(t *testing.T) {
	lsm := &LogSessionManager{
		activeSessions:    make(map[string]*LogSessionInfo),
		webSocketHandlers: make(map[string]*LogSessionWebSocketHandler),
	}

	expiredID := "log-session-expired"
	freshID := "log-session-fresh"
	lsm.activeSessions[expiredID] = &LogSessionInfo{
		Session:            &LogSession{SessionID: expiredID},
		IsStreaming:        true,
		streamingStartedAt: time.Now().Add(-25 * time.Hour),
	}
	lsm.activeSessions[freshID] = &LogSessionInfo{
		Session:            &LogSession{SessionID: freshID},
		IsStreaming:        true,
		streamingStartedAt: time.Now().Add(-1 * time.Hour),
	}

	lsm.expireOldLogSessionsLocked()

	if _, ok := lsm.activeSessions[expiredID]; ok {
		t.Fatal("expected expired session to be removed")
	}
	if _, ok := lsm.activeSessions[freshID]; !ok {
		t.Fatal("expected fresh session to remain")
	}
}

func TestLogSessionManager_markStreamingStarted(t *testing.T) {
	info := &LogSessionInfo{}
	info.markStreamingStarted()
	if !info.IsStreaming {
		t.Fatal("expected IsStreaming=true")
	}
	if info.streamingStartedAt.IsZero() {
		t.Fatal("expected streamingStartedAt to be set")
	}
}

func TestLogHandler_SendMessageBuffersWhenDisconnected(t *testing.T) {
	h := newTestLogHandler("ws://unused", "log-buffer")
	h.isConnected.Store(false)
	h.isActive.Store(false)

	if err := h.SendMessage(LogTypeLogLine, []byte("hello")); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if h.bufferedFrames.Load() != 1 {
		t.Fatalf("bufferedFrames = %d, want 1", h.bufferedFrames.Load())
	}
}
