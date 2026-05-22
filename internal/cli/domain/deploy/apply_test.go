package deploy

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eclipse-iofog/agent/internal/cli/run"
	"github.com/eclipse-iofog/agent/internal/cli/ui"
)

type applyFakeAPI struct {
	multipartPath string
	startResult   map[string]interface{}
	statusCalls   []map[string]interface{}
	statusIndex   int
}

func (f *applyFakeAPI) RequestV3(method, path string, _ interface{}) (map[string]interface{}, error) {
	if strings.Contains(path, ":apply/") {
		if f.statusIndex >= len(f.statusCalls) {
			return f.statusCalls[len(f.statusCalls)-1], nil
		}
		resp := f.statusCalls[f.statusIndex]
		f.statusIndex++
		return resp, nil
	}
	return map[string]interface{}{}, nil
}

func (f *applyFakeAPI) RequestV3MultipartFile(method, path, _, filePath string, fields map[string]string) (map[string]interface{}, error) {
	f.multipartPath = path
	if fields["dryRun"] == "true" {
		return map[string]interface{}{"valid": true, "kind": "Microservice", "name": "demo", "apiVersion": "v3"}, nil
	}
	return f.startResult, nil
}

func (f *applyFakeAPI) IsDaemonRunning() bool { return true }

func TestExecute_DryRunDoesNotPoll(t *testing.T) {
	manifest := writeManifest(t, "kind: Microservice\napiVersion: v3\nname: demo\n")
	api := &applyFakeAPI{}
	_, err := Execute(context.Background(), api, ui.New(ui.Options{}), Request{
		ManifestPath: manifest,
		DryRun:       true,
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(api.multipartPath, ":validate") {
		t.Fatalf("expected validate path, got %q", api.multipartPath)
	}
	if api.statusIndex != 0 {
		t.Fatalf("expected no polling, status calls=%d", api.statusIndex)
	}
}

func TestExecute_MicroserviceApplyCollectsStages(t *testing.T) {
	manifest := writeManifest(t, "kind: Microservice\napiVersion: v3\nname: demo\n")
	api := &applyFakeAPI{
		startResult: map[string]interface{}{"status": "running", "operationId": "op-1"},
		statusCalls: []map[string]interface{}{
			{"status": "running", "stage": "persisting"},
			{"status": "running", "stage": "pulling"},
			{"status": "succeeded", "deploymentId": "dep-99"},
		},
	}
	prevInterval := runtimeClassApplyPollInterval
	runtimeClassApplyPollInterval = time.Millisecond
	t.Cleanup(func() { runtimeClassApplyPollInterval = prevInterval })

	result, err := Execute(context.Background(), api, ui.New(ui.Options{}), Request{ManifestPath: manifest})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(result.Stages) != 2 {
		t.Fatalf("expected stages, got %#v", result.Stages)
	}
	stages, ok := result.Data["stages"].([]interface{})
	if !ok || len(stages) != 2 {
		t.Fatalf("expected stages array in data, got %#v", result.Data["stages"])
	}
}

func TestExecute_RuntimeClassApplyPollSucceeded(t *testing.T) {
	manifest := writeManifest(t, "kind: RuntimeClass\napiVersion: v3\nname: edgelet\n")
	prevStart := startMultipartApply
	prevStatus := fetchApplyStatus
	prevInterval := runtimeClassApplyPollInterval
	t.Cleanup(func() {
		startMultipartApply = prevStart
		fetchApplyStatus = prevStatus
		runtimeClassApplyPollInterval = prevInterval
	})
	runtimeClassApplyPollInterval = time.Millisecond

	polls := 0
	startMultipartApply = func(api run.V3Client, target Target, _ string, _ map[string]string) (map[string]interface{}, error) {
		if target != TargetRuntimeClasses {
			t.Fatalf("expected runtimeclasses target, got %s", target)
		}
		return map[string]interface{}{"status": "running", "operationId": "op-rc"}, nil
	}
	fetchApplyStatus = func(_ run.V3Client, target Target, operationID string) (map[string]interface{}, error) {
		if target != TargetRuntimeClasses || operationID != "op-rc" {
			t.Fatalf("unexpected status fetch: %s %s", target, operationID)
		}
		polls++
		if polls < 2 {
			return map[string]interface{}{"status": "running", "stage": "reconfiguring"}, nil
		}
		return map[string]interface{}{
			"status": "succeeded",
			"runtimeClass": map[string]interface{}{
				"name":    "edgelet",
				"handler": "edgelet-wasm",
			},
		}, nil
	}

	result, err := Execute(context.Background(), &applyFakeAPI{}, ui.New(ui.Options{}), Request{ManifestPath: manifest})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(result.Human, "runtimeclass manifest applied successfully") {
		t.Fatalf("unexpected human output: %s", result.Human)
	}
}

func writeManifest(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "manifest.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
