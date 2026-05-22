package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eclipse-iofog/agent/internal/cli/run"
)

func writeTestManifest(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "manifest.yaml")
	content := "kind: Microservice\napiVersion: v3\nname: demo\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func (f *fakeClient) RequestV3MultipartFile(method, path, fileField, filePath string, fields map[string]string) (map[string]interface{}, error) {
	switch {
	case strings.Contains(path, ":validate"):
		return map[string]interface{}{"valid": true, "kind": "Microservice", "name": "demo", "apiVersion": "v3"}, nil
	case strings.Contains(path, ":apply") && !strings.Contains(path, ":apply/"):
		return map[string]interface{}{"status": "running", "operationId": "op-1"}, nil
	case strings.Contains(path, ":apply/op-1"):
		return map[string]interface{}{"status": "succeeded", "deploymentId": "dep-1", "stage": "done"}, nil
	default:
		return map[string]interface{}{}, nil
	}
}

func TestDeployDryRunJSON(t *testing.T) {
	manifest := writeTestManifest(t)
	client := &fakeClient{running: true}
	stdout, _, code := runCLI(t, client, "deploy", "-f", manifest, "--dry-run", "-o", "json")
	if code != 0 {
		t.Fatalf("exit=%d stdout=%q", code, stdout)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &decoded); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if decoded["valid"] != true {
		t.Fatalf("expected validate payload, got %#v", decoded)
	}
}

func TestDeployApplyJSONIncludesStages(t *testing.T) {
	manifest := writeTestManifest(t)
	client := &fakeClient{running: true}
	stdout, _, code := runCLI(t, client, "deploy", "-f", manifest, "-o", "json")
	if code != 0 {
		t.Fatalf("exit=%d stdout=%q", code, stdout)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &decoded); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if _, ok := decoded["stages"]; !ok {
		t.Fatalf("expected stages in json output, got %#v", decoded)
	}
}

func TestDeployApplyHumanSuccessOnStderr(t *testing.T) {
	manifest := writeTestManifest(t)
	client := &fakeClient{running: true}
	stdout, stderr, code := runCLI(t, client, "deploy", "-f", manifest)
	if code != 0 {
		t.Fatalf("exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if strings.TrimSpace(stdout) != "" {
		t.Fatalf("expected empty stdout for human deploy success, got %q", stdout)
	}
	if !strings.Contains(stderr, "✔") || !strings.Contains(stderr, "microservice manifest applied successfully") {
		t.Fatalf("expected success marker on stderr, got stderr=%q", stderr)
	}
}

func TestDeployApplyHumanNoColorPlainMarker(t *testing.T) {
	manifest := writeTestManifest(t)
	client := &fakeClient{running: true}
	stdout, stderr, code := runCLI(t, client, "deploy", "-f", manifest, "--no-color")
	if code != 0 {
		t.Fatalf("exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if strings.TrimSpace(stdout) != "" {
		t.Fatalf("expected empty stdout, got %q", stdout)
	}
	if !strings.Contains(stderr, "✔ microservice manifest applied successfully") {
		t.Fatalf("expected plain success marker, got stderr=%q", stderr)
	}
	if strings.Contains(stderr, "\x1b[") {
		t.Fatalf("expected no ANSI escapes with --no-color, got stderr=%q", stderr)
	}
}

func TestDeployApplySubcommandRejected(t *testing.T) {
	client := &fakeClient{running: true}
	_, _, code := runCLI(t, client, "deploy", "apply", "-f", "/tmp/manifest.yaml")
	if code != run.ExitInvalidArgument {
		t.Fatalf("expected exit 2, got %d", code)
	}
}

func TestDeployRegistryPrefixRejected(t *testing.T) {
	client := &fakeClient{running: true}
	_, _, code := runCLI(t, client, "deploy", "registry", "-f", "/tmp/manifest.yaml")
	if code != run.ExitInvalidArgument {
		t.Fatalf("expected exit 2, got %d", code)
	}
}
