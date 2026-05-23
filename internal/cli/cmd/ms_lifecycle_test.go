package cmd

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMSStartHumanShowsSpinnerAndSuccessMarker(t *testing.T) {
	client := &fakeClient{
		running: true,
		gets: map[string]map[string]interface{}{
			"POST /v3/ms/ms-1/start": {
				"status":           "ok",
				"microserviceUuid": "ms-1",
			},
		},
	}
	stdout, stderr, code := runCLI(t, client, "ms", "start", "ms-1")
	if code != 0 {
		t.Fatalf("exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "Starting microservice ms-1...") {
		t.Fatalf("expected spinner message on stderr, got stderr=%q", stderr)
	}
	if !strings.Contains(stderr, "✔ microservice start completed successfully") {
		t.Fatalf("expected success marker on stderr, got stderr=%q", stderr)
	}
	if strings.TrimSpace(stdout) != "" {
		t.Fatalf("expected empty stdout for single-line lifecycle success, got stdout=%q", stdout)
	}
}

func TestMSStartHumanWritesDetailToStdout(t *testing.T) {
	client := &fakeClient{
		running: true,
		gets: map[string]map[string]interface{}{
			"POST /v3/ms/ms-1/start": {
				"status":           "ok",
				"microserviceUuid": "ms-1",
				"warning":          "controller reconcile may restart it",
			},
		},
	}
	stdout, stderr, code := runCLI(t, client, "ms", "start", "ms-1")
	if code != 0 {
		t.Fatalf("exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "✔ microservice start completed successfully") {
		t.Fatalf("expected summary marker on stderr, got stderr=%q", stderr)
	}
	if strings.Contains(stderr, "warning:") {
		t.Fatalf("expected warning detail off stderr, got stderr=%q", stderr)
	}
	if !strings.Contains(stdout, "warning: controller reconcile may restart it") {
		t.Fatalf("expected warning detail on stdout, got stdout=%q", stdout)
	}
}

func TestMSStartJSONStdoutOnly(t *testing.T) {
	client := &fakeClient{
		running: true,
		gets: map[string]map[string]interface{}{
			"POST /v3/ms/ms-1/start": {
				"status":           "ok",
				"microserviceUuid": "ms-1",
			},
		},
	}
	stdout, stderr, code := runCLI(t, client, "-o", "json", "ms", "start", "ms-1")
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
	if decoded["microserviceUuid"] != "ms-1" {
		t.Fatalf("unexpected payload: %#v", decoded)
	}
}
