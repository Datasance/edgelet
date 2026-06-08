package cmd

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestControlPlaneGetJSON(t *testing.T) {
	client := &fakeClient{
		running: true,
		gets: map[string]map[string]any{
			"GET /v1/system/controlplane": {
				"controllerUuid": "cp-uuid-1",
				"namespace":      "default",
				"name":           "pot",
				"image":          "ghcr.io/datasance/controller:3.7.0",
				"state":          "running",
				"runtimeState":   "running",
				"source":         "controlplane",
				"type":           "controlplane",
			},
		},
	}
	stdout, _, code := runCLI(t, client, "controlplane", "get", "-o", "json")
	if code != 0 {
		t.Fatalf("exit=%d stdout=%q", code, stdout)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &decoded); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if decoded["controllerUuid"] != "cp-uuid-1" || decoded["namespace"] != "default" || decoded["name"] != "pot" {
		t.Fatalf("unexpected payload: %#v", decoded)
	}
}

func TestControlPlaneGetManifestHuman(t *testing.T) {
	client := &fakeClient{
		running: true,
		gets: map[string]map[string]any{
			"GET /v1/system/controlplane/manifest": {
				"manifestYaml": "kind: ControlPlane\nmetadata:\n  name: pot\n",
				"masked":       true,
			},
		},
	}
	stdout, _, code := runCLI(t, client, "controlplane", "get", "--manifest")
	if code != 0 {
		t.Fatalf("exit=%d stdout=%q", code, stdout)
	}
	if !strings.Contains(stdout, "manifestYaml (secrets masked):") {
		t.Fatalf("expected masked header, got stdout=%q", stdout)
	}
	if !strings.Contains(stdout, "kind: ControlPlane") {
		t.Fatalf("expected manifest body, got stdout=%q", stdout)
	}
}

func TestControlPlaneGetHumanShowsNamespaceAndName(t *testing.T) {
	client := &fakeClient{
		running: true,
		gets: map[string]map[string]any{
			"GET /v1/system/controlplane": {
				"controllerUuid": "cp-uuid-1",
				"namespace":      "bar",
				"name":           "foo",
				"state":          "running",
			},
		},
	}
	stdout, _, code := runCLI(t, client, "controlplane", "get")
	if code != 0 {
		t.Fatalf("exit=%d stdout=%q", code, stdout)
	}
	if !strings.Contains(stdout, "namespace: bar") || !strings.Contains(stdout, "name: foo") {
		t.Fatalf("expected namespace and name in human output, got stdout=%q", stdout)
	}
}

func TestControlPlaneDeleteHuman(t *testing.T) {
	client := &fakeClient{
		running: true,
		gets: map[string]map[string]any{
			"DELETE /v1/system/controlplane": {"status": "ok"},
		},
	}
	stdout, _, code := runCLI(t, client, "controlplane", "delete")
	if code != 0 {
		t.Fatalf("exit=%d stdout=%q", code, stdout)
	}
	if !strings.Contains(stdout, "control plane deployment removed successfully") {
		t.Fatalf("expected delete success message, got stdout=%q", stdout)
	}
}

func TestControlPlaneHelpAndRootListing(t *testing.T) {
	client := &fakeClient{running: true}
	stdout, _, code := runCLI(t, client, "controlplane", "--help")
	if code != 0 {
		t.Fatalf("controlplane --help exit=%d", code)
	}
	if !strings.Contains(stdout, "controlplane get") || !strings.Contains(stdout, "--manifest") {
		t.Fatalf("expected get/--manifest in help, got stdout=%q", stdout)
	}
	if !strings.Contains(stdout, "deploy -f") {
		t.Fatalf("expected deploy -f hint in Long, got stdout=%q", stdout)
	}

	rootOut, _, code := runCLI(t, client, "--help")
	if code != 0 {
		t.Fatalf("root --help exit=%d", code)
	}
	if !strings.Contains(rootOut, "controlplane") {
		t.Fatalf("expected controlplane in root help, got stdout=%q", rootOut)
	}
}

func TestDeployHelpMentionsControlPlane(t *testing.T) {
	client := &fakeClient{running: true}
	stdout, _, code := runCLI(t, client, "deploy", "--help")
	if code != 0 {
		t.Fatalf("deploy --help exit=%d", code)
	}
	if !strings.Contains(stdout, "ControlPlane") {
		t.Fatalf("expected ControlPlane in deploy help, got stdout=%q", stdout)
	}
	if !strings.Contains(stdout, "controlplane.yaml") {
		t.Fatalf("expected controlplane.yaml example, got stdout=%q", stdout)
	}
}
