//go:build linux

package runtimecmd

import (
	"errors"
	"testing"
	"time"

	"github.com/eclipse-iofog/edgelet/internal/cli/run"
)

type runtimeDrainMockClient struct {
	lastMethod string
	lastPath   string
	lastBody   map[string]any
	response   map[string]any
	err        error
}

func (m *runtimeDrainMockClient) Request(method, path string, body any) (map[string]any, error) {
	m.lastMethod = method
	m.lastPath = path
	if typed, ok := body.(map[string]any); ok {
		m.lastBody = typed
	}
	if m.err != nil {
		return nil, m.err
	}
	return m.response, nil
}

func (m *runtimeDrainMockClient) RequestMultipartFile(_, _, _, _ string, _ map[string]string) (map[string]any, error) {
	return nil, nil
}

func (m *runtimeDrainMockClient) IsDaemonRunning() bool { return true }

func TestDrain_StopsViaEdgeletAPI(t *testing.T) {
	client := &runtimeDrainMockClient{
		response: map[string]any{"status": "ok"},
	}

	result, err := Drain(client, 60)
	if err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if client.lastMethod != "POST" || client.lastPath != drainRoute {
		t.Fatalf("unexpected request: %s %s", client.lastMethod, client.lastPath)
	}
	if client.lastBody["timeoutSeconds"] != 60 {
		t.Fatalf("timeoutSeconds=%v", client.lastBody["timeoutSeconds"])
	}
	if result.Human == "" {
		t.Fatal("expected human success message")
	}
}

func TestDrain_NilClient(t *testing.T) {
	_, err := Drain(nil, 30)
	if err == nil {
		t.Fatal("expected error for nil client")
	}
	var cliErr *run.CLIError
	if !errors.As(err, &cliErr) || cliErr.Code != run.CodeInternal {
		t.Fatalf("expected internal CLI error, got %v", err)
	}
}

func TestDrainHTTPClientTimeout_CoversServerBudget(t *testing.T) {
	if got := DrainHTTPClientTimeout(90); got != 105*time.Second {
		t.Fatalf("expected 105s client budget for 90s drain, got %v", got)
	}
	if got := DrainHTTPClientTimeout(0); got != 105*time.Second {
		t.Fatalf("expected default 105s client budget, got %v", got)
	}
}
