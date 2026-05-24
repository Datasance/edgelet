package cmd

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSpinnerUX_ProvisionQuietNoSpinnerControlChars(t *testing.T) {
	client := &fakeClient{
		running: true,
		gets: map[string]map[string]interface{}{
			"POST /v1/system/provision": {"agentUuid": "agent-1"},
		},
	}
	_, stderr, code := runCLI(t, client, "--quiet", "provision", "test-key")
	if code != 0 {
		t.Fatalf("exit=%d stderr=%q", code, stderr)
	}
	if strings.Contains(stderr, "\r") {
		t.Fatalf("expected no spinner control output with --quiet, got stderr=%q", stderr)
	}
	if !strings.Contains(stderr, "Provisioning agent...") {
		t.Fatalf("expected plain progress line on stderr, got stderr=%q", stderr)
	}
}

func TestSpinnerUX_MSStartQuietNoSpinnerControlChars(t *testing.T) {
	client := &fakeClient{
		running: true,
		gets: map[string]map[string]interface{}{
			"POST /v1/ms/ms-1/start": {
				"status":           "ok",
				"microserviceUuid": "ms-1",
			},
		},
	}
	_, stderr, code := runCLI(t, client, "--quiet", "ms", "start", "ms-1")
	if code != 0 {
		t.Fatalf("exit=%d stderr=%q", code, stderr)
	}
	if strings.Contains(stderr, "\r") {
		t.Fatalf("expected no spinner control output with --quiet, got stderr=%q", stderr)
	}
	if !strings.Contains(stderr, "Starting microservice ms-1...") {
		t.Fatalf("expected plain progress line on stderr, got stderr=%q", stderr)
	}
}

func TestSpinnerUX_ImagePullHumanSuccessOnStderr(t *testing.T) {
	client := &fakeClient{running: true}
	stdout, stderr, code := runCLI(t, client, "image", "pull", "docker.io/library/alpine:3.19")
	if code != 0 {
		t.Fatalf("exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "Pulling image...") {
		t.Fatalf("expected spinner message on stderr, got stderr=%q", stderr)
	}
	if !strings.Contains(stderr, "✔ image pulled successfully") {
		t.Fatalf("expected success marker on stderr, got stderr=%q", stderr)
	}
	if strings.TrimSpace(stdout) != "" {
		t.Fatalf("expected empty stdout for human image pull success, got stdout=%q", stdout)
	}
}

func TestSpinnerUX_ImagePullJSONStdoutOnly(t *testing.T) {
	client := &fakeClient{running: true}
	stdout, stderr, code := runCLI(t, client, "-o", "json", "image", "pull", "docker.io/library/alpine:3.19")
	if code != 0 {
		t.Fatalf("exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if strings.TrimSpace(stderr) != "" {
		t.Fatalf("expected no stderr UX for json output, got %q", stderr)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &decoded); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if decoded["status"] != "succeeded" {
		t.Fatalf("unexpected payload: %#v", decoded)
	}
}

func TestSpinnerUX_ImagePullQuietNoSpinnerControlChars(t *testing.T) {
	client := &fakeClient{running: true}
	_, stderr, code := runCLI(t, client, "--quiet", "image", "pull", "docker.io/library/alpine:3.19")
	if code != 0 {
		t.Fatalf("exit=%d stderr=%q", code, stderr)
	}
	if strings.Contains(stderr, "\r") {
		t.Fatalf("expected no spinner control output with --quiet, got stderr=%q", stderr)
	}
	if !strings.Contains(stderr, "Pulling image...") {
		t.Fatalf("expected plain progress line on stderr, got stderr=%q", stderr)
	}
}
