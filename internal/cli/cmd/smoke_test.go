package cmd

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/eclipse-iofog/agent/internal/cli/run"
)

func TestExitCodeMatrix(t *testing.T) {
	cases := []struct {
		code string
		want int
	}{
		{run.CodeInvalidArgument, run.ExitInvalidArgument},
		{run.CodeUnauthorized, run.ExitUnauthorized},
		{run.CodeForbidden, run.ExitUnauthorized},
		{run.CodeNotFound, run.ExitNotFound},
		{run.CodeConflict, run.ExitConflict},
		{run.CodeNotImplemented, run.ExitNotImplemented},
		{run.CodeDaemonUnavailable, run.ExitDaemonUnavailable},
		{run.CodeInternal, run.ExitInternal},
		{"", run.ExitInternal},
	}
	for _, tc := range cases {
		if got := run.ExitCodeForCode(tc.code); got != tc.want {
			t.Fatalf("ExitCodeForCode(%q) = %d, want %d", tc.code, got, tc.want)
		}
	}
}

func TestLegacyCommandRejectionSuite(t *testing.T) {
	client := &fakeClient{running: true}
	cases := []struct {
		name          string
		args          []string
		want          int // 0 means any non-zero
	}{
		{"top-level status", []string{"status"}, 0},
		{"top-level info", []string{"info"}, 0},
		{"top-level start", []string{"start"}, run.ExitInvalidArgument},
		{"top-level stop", []string{"stop"}, run.ExitInvalidArgument},
		{"top-level cert", []string{"cert", "abc"}, run.ExitInvalidArgument},
		{"top-level switch", []string{"switch", "dev"}, run.ExitInvalidArgument},
		{"ms ps", []string{"ms", "ps"}, run.ExitInvalidArgument},
		{"deploy apply", []string{"deploy", "apply", "-f", "/tmp/x.yaml"}, run.ExitInvalidArgument},
		{"deploy registry prefix", []string{"deploy", "registry", "-f", "/tmp/x.yaml"}, run.ExitInvalidArgument},
		{"config set", []string{"config", "set", "k", "v"}, run.ExitInvalidArgument},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, code := runCLI(t, client, tc.args...)
			if tc.want == 0 {
				if code == 0 {
					t.Fatalf("expected non-zero exit for legacy command")
				}
				return
			}
			if code != tc.want {
				t.Fatalf("expected exit %d, got %d", tc.want, code)
			}
		})
	}
}

func TestDaemonDownExitCode10Smoke(t *testing.T) {
	client := &fakeClient{running: false}
	_, _, code := runCLI(t, client, "system", "status", "-o", "json")
	if code != run.ExitDaemonUnavailable {
		t.Fatalf("expected exit 10, got %d", code)
	}
}

func TestMSListJSONJqFriendly(t *testing.T) {
	client := &fakeClient{
		running: true,
		gets: map[string]map[string]interface{}{
			"GET /v3/ms?source=all": {
				"items": []interface{}{
					map[string]interface{}{
						"uuid":        "abc-123",
						"application": "local",
						"name":        "demo",
						"state":       "running",
						"containerId": "c1",
						"image":       "demo:latest",
						"type":        "standard",
					},
				},
			},
		},
	}
	stdout, stderr, code := runCLI(t, client, "ms", "ls", "-o", "json")
	if code != 0 {
		t.Fatalf("exit=%d stderr=%q", code, stderr)
	}
	if strings.TrimSpace(stderr) != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
	var decoded struct {
		Items []struct {
			UUID  string `json:"uuid"`
			Name  string `json:"name"`
			State string `json:"state"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &decoded); err != nil {
		t.Fatalf("stdout is not jq-friendly json: %v (%q)", err, stdout)
	}
	if len(decoded.Items) != 1 || decoded.Items[0].UUID != "abc-123" {
		t.Fatalf("unexpected payload: %#v", decoded)
	}
}

func TestSystemStatusJSONJqFriendly(t *testing.T) {
	client := &fakeClient{
		running: true,
		gets: map[string]map[string]interface{}{
			"GET /v3/system/status": {
				"iofogDaemon":            "running",
				"connectionToController": "ok",
				"cpuUsage":               float64(12),
			},
		},
	}
	stdout, _, code := runCLI(t, client, "system", "status", "-o", "json")
	if code != 0 {
		t.Fatalf("exit=%d", code)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &decoded); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if decoded["iofogDaemon"] != "running" {
		t.Fatalf("unexpected payload: %#v", decoded)
	}
}
