package fieldagent

import (
	"testing"

	"github.com/eclipse-iofog/edgelet/internal/models"
)

func TestHandleExecSessions_DisabledResetsWebSocketWithoutTrackedExec(t *testing.T) {
	msUUID := "ms-no-exec"
	handler := newTestExecHandler("ws://unused", msUUID)
	handler.isConnected.Store(false)
	handler.state.Store(StatePending)
	activeExecHandlers.Store(msUUID, handler)

	fa := &FieldAgent{
		activeExecSessions: map[string]string{},
		execCallbacks:      map[string]*ExecSessionCallback{},
		activeWebSockets:   map[string]*ExecSessionWebSocketHandler{},
	}

	fa.HandleExecSessions([]*models.Microservice{{
		MicroserviceUUID: msUUID,
		ExecEnabled:      false,
	}})

	if got := handler.GetConnectionState(); got != StateDisconnected {
		t.Fatalf("handler state = %v, want StateDisconnected after exec disabled cleanup", got)
	}
}

func TestResetExecWebSocketHandler_ClearsActiveWebSockets(t *testing.T) {
	msUUID := "ms-reset-map"
	handler := newTestExecHandler("ws://unused", msUUID)
	handler.state.Store(StatePending)

	fa := &FieldAgent{
		activeExecSessions: map[string]string{},
		execCallbacks:      map[string]*ExecSessionCallback{},
		activeWebSockets:   map[string]*ExecSessionWebSocketHandler{msUUID: handler},
	}
	activeExecHandlers.Store(msUUID, handler)

	fa.resetExecWebSocketHandler(msUUID)

	fa.execSessionsMu.RLock()
	_, exists := fa.activeWebSockets[msUUID]
	fa.execSessionsMu.RUnlock()
	if exists {
		t.Fatal("expected handler removed from activeWebSockets")
	}
	if got := handler.GetConnectionState(); got != StateDisconnected {
		t.Fatalf("handler state = %v, want StateDisconnected", got)
	}
}
