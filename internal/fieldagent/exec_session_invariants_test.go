package fieldagent

import "testing"

func TestHandleExecSessionClose_RemovesMatchingActiveSession(t *testing.T) {
	fa := &FieldAgent{
		activeExecSessions: map[string]string{"ms-1": "exec-1"},
		execCallbacks:      map[string]*ExecSessionCallback{},
		activeWebSockets:   map[string]*ExecSessionWebSocketHandler{},
	}

	if err := fa.HandleExecSessionClose("ms-1", "exec-1"); err != nil {
		t.Fatalf("expected nil error on close, got %v", err)
	}

	if got := fa.GetActiveExecSession("ms-1"); got != "" {
		t.Fatalf("expected active exec session to be removed, got %q", got)
	}
}

func TestHandleExecSessionClose_KeepsNonMatchingSessionID(t *testing.T) {
	fa := &FieldAgent{
		activeExecSessions: map[string]string{"ms-1": "exec-keep"},
		execCallbacks:      map[string]*ExecSessionCallback{},
		activeWebSockets:   map[string]*ExecSessionWebSocketHandler{},
	}

	if err := fa.HandleExecSessionClose("ms-1", "exec-other"); err != nil {
		t.Fatalf("expected nil error on close, got %v", err)
	}

	if got := fa.GetActiveExecSession("ms-1"); got != "exec-keep" {
		t.Fatalf("expected non-matching session id to remain, got %q", got)
	}
}
