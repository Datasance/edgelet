package handlers

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/datasance/edgelet/pkg/engine"
	"github.com/gorilla/websocket"
)

func TestHandleLogsStreamWS_UsesFollowStreamAndForwardsEntries(t *testing.T) {
	handler := NewV3Handler()
	handler.resolveMicroservice = func(selector string) (string, error) {
		if selector != "local.demo" {
			t.Fatalf("unexpected selector: %s", selector)
		}
		return "ms-1", nil
	}
	var gotCfg *engine.TailConfig
	handler.streamMicroservicLog = func(microserviceUUID string, cfg *engine.TailConfig, tailHandler engine.LogTailHandler) error {
		if microserviceUUID != "ms-1" {
			t.Fatalf("unexpected microservice uuid: %s", microserviceUUID)
		}
		gotCfg = cfg
		tailHandler.OnLogLine("sid", microserviceUUID, []byte("hello"), engine.Stdout)
		tailHandler.OnLogLine("sid", microserviceUUID, []byte("hello"), engine.Stdout)
		tailHandler.OnComplete("sid")
		return nil
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.handleLogsStreamWS(w, r, "local.demo")
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "?tail=25&since=2026-01-01T00:00:00Z&until=2026-01-01T01:00:00Z"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to dial websocket: %v", err)
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))

	var ev1 map[string]interface{}
	if err := conn.ReadJSON(&ev1); err != nil {
		t.Fatalf("failed to read first event: %v", err)
	}
	var ev2 map[string]interface{}
	if err := conn.ReadJSON(&ev2); err != nil {
		t.Fatalf("failed to read second event: %v", err)
	}
	if gotCfg == nil {
		t.Fatalf("expected tail config to be passed")
	}
	if !gotCfg.Follow || gotCfg.Lines != 25 || gotCfg.Since != "2026-01-01T00:00:00Z" || gotCfg.Until != "2026-01-01T01:00:00Z" {
		t.Fatalf("unexpected tail config: %+v", gotCfg)
	}
	if ev1["line"] != "hello" || ev2["line"] != "hello" {
		t.Fatalf("expected forwarded log lines, got ev1=%v ev2=%v", ev1, ev2)
	}
}

func TestHandleLogsStreamWS_InvalidTailReturnsErrorEvent(t *testing.T) {
	handler := NewV3Handler()
	handler.resolveMicroservice = func(_ string) (string, error) { return "ms-1", nil }
	streamCalled := false
	handler.streamMicroservicLog = func(_ string, _ *engine.TailConfig, _ engine.LogTailHandler) error {
		streamCalled = true
		return nil
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.handleLogsStreamWS(w, r, "local.demo")
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "?tail=bad"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to dial websocket: %v", err)
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))

	var ev map[string]interface{}
	if err := conn.ReadJSON(&ev); err != nil {
		t.Fatalf("failed to read error event: %v", err)
	}
	if !strings.Contains(stringValue(ev["error"]), "invalid tail parameter") {
		t.Fatalf("expected invalid tail error, got: %v", ev)
	}
	if streamCalled {
		t.Fatalf("expected stream function not to be called on invalid tail")
	}
}

func TestHandleLogsStreamWS_StreamingErrorReturnsErrorEvent(t *testing.T) {
	handler := NewV3Handler()
	handler.resolveMicroservice = func(_ string) (string, error) { return "ms-1", nil }
	handler.streamMicroservicLog = func(_ string, _ *engine.TailConfig, _ engine.LogTailHandler) error {
		return errors.New("stream init failed")
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.handleLogsStreamWS(w, r, "local.demo")
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to dial websocket: %v", err)
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))

	var ev map[string]interface{}
	if err := conn.ReadJSON(&ev); err != nil {
		t.Fatalf("failed to read error event: %v", err)
	}
	if !strings.Contains(stringValue(ev["error"]), "stream init failed") {
		t.Fatalf("expected stream init error, got: %v", ev)
	}
}

func TestHandleSystemLogsStreamWS_StreamsDaemonLogs(t *testing.T) {
	cfg := setupConfigForGPSTests(t)
	cfg.LogDiskDirectory = t.TempDir() + string(os.PathSeparator)
	logFile := filepath.Join(cfg.LogDiskDirectory, "iofog-agent.0.log")
	if err := os.WriteFile(logFile, []byte("2026-05-17 00:00:01.000 [info] boot\n"), 0o600); err != nil {
		t.Fatalf("failed to write log file: %v", err)
	}

	handler := NewV3Handler()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.handleSystemLogsStreamWS(w, r)
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "?tailLines=1"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to dial websocket: %v", err)
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))

	var ev map[string]interface{}
	if err := conn.ReadJSON(&ev); err != nil {
		t.Fatalf("failed to read log event: %v", err)
	}
	if !strings.Contains(stringValue(ev["line"]), "boot") {
		t.Fatalf("expected streamed daemon log line, got: %v", ev)
	}
}

func TestHandleSystemLogsStreamWS_InvalidTailReturnsErrorEvent(t *testing.T) {
	handler := NewV3Handler()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.handleSystemLogsStreamWS(w, r)
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "?tailLines=bad"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to dial websocket: %v", err)
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))

	var ev map[string]interface{}
	if err := conn.ReadJSON(&ev); err != nil {
		t.Fatalf("failed to read error event: %v", err)
	}
	if !strings.Contains(stringValue(ev["error"]), "invalid tailLines parameter") {
		t.Fatalf("expected invalid tailLines error, got: %v", ev)
	}
}

func stringValue(v interface{}) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
