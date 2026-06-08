package cmd

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestProvisionHumanShowsSpinnerAndSuccessMarker(t *testing.T) {
	client := &fakeClient{
		running: true,
		gets: map[string]map[string]any{
			"POST /v1/system/provision": {"agentUuid": "agent-1"},
		},
	}
	stdout, stderr, code := runCLI(t, client, "provision", "test-key")
	if code != 0 {
		t.Fatalf("exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "Provisioning agent...") {
		t.Fatalf("expected spinner message on stderr, got stderr=%q", stderr)
	}
	if !strings.Contains(stderr, "✔ agent provisioned successfully") {
		t.Fatalf("expected success marker on stderr, got stderr=%q", stderr)
	}
	if strings.TrimSpace(stdout) != "" {
		t.Fatalf("expected empty stdout for single-line provision success, got stdout=%q", stdout)
	}
}

func TestProvisionJSONStdoutOnly(t *testing.T) {
	client := &fakeClient{
		running: true,
		gets: map[string]map[string]any{
			"POST /v1/system/provision": {"agentUuid": "agent-1"},
		},
	}
	stdout, stderr, code := runCLI(t, client, "-o", "json", "provision", "test-key")
	if code != 0 {
		t.Fatalf("exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if strings.TrimSpace(stderr) != "" {
		t.Fatalf("expected no stderr UX for json output, got %q", stderr)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &decoded); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if decoded["agentUuid"] != "agent-1" {
		t.Fatalf("unexpected payload: %#v", decoded)
	}
}

func TestDeprovisionHumanShowsSuccessMarker(t *testing.T) {
	client := &fakeClient{
		running: true,
		gets: map[string]map[string]any{
			"DELETE /v1/system/provision": {"status": "ok"},
		},
	}
	stdout, stderr, code := runCLI(t, client, "deprovision")
	if code != 0 {
		t.Fatalf("exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "✔ agent deprovisioned successfully") {
		t.Fatalf("expected success marker on stderr, got stderr=%q", stderr)
	}
	if strings.TrimSpace(stdout) != "" {
		t.Fatalf("expected empty stdout for single-line deprovision success, got stdout=%q", stdout)
	}
}
