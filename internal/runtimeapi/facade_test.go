package runtimeapi

import (
	"strings"
	"testing"

	"github.com/eclipse-iofog/agent/internal/models"
)

func testLocalManifestYAML() string {
	return strings.TrimSpace(`
apiVersion: datasance.com/v3
kind: Microservice
metadata:
  name: router
spec:
  images:
    x86: quay.io/skupper/skupper-router:latest
`) + "\n"
}

func testLocalManifestWithLabelsYAML() string {
	return strings.TrimSpace(`
apiVersion: iofog.org/v3
kind: Microservice
metadata:
  name: svc-a
  labels:
    team: edge
    owner: runtime
spec:
  images:
    x86: nginx:latest
`) + "\n"
}

func TestFacadePullImage_ValidatesRequiredImage(t *testing.T) {
	f := NewFacade()
	if _, err := f.PullImage("   ", nil, ""); err == nil || !strings.Contains(err.Error(), "image is required") {
		t.Fatalf("expected required image validation error, got: %v", err)
	}
}

func TestFacadePullImage_ValidatesPlatform(t *testing.T) {
	f := NewFacade()
	if _, err := f.PullImage("nginx:latest", nil, "bad-platform"); err == nil || !strings.Contains(err.Error(), "platform must follow") {
		t.Fatalf("expected platform validation error, got: %v", err)
	}
}

func TestFacadeLoadImageFromPath_ValidatesPath(t *testing.T) {
	f := NewFacade()
	if _, err := f.LoadImageFromPath("   "); err == nil || !strings.Contains(err.Error(), "path is required") {
		t.Fatalf("expected missing path validation error, got: %v", err)
	}
}

func TestFacadeLoadImageFromPath_RequiresRegularFile(t *testing.T) {
	f := NewFacade()
	if _, err := f.LoadImageFromPath(t.TempDir()); err == nil || !strings.Contains(err.Error(), "regular file") {
		t.Fatalf("expected regular file validation error, got: %v", err)
	}
}

func TestFacadePullImage_RejectsFromCacheRegistry(t *testing.T) {
	f := NewFacade()
	if err := f.db.Open(t.TempDir()); err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	t.Cleanup(func() { _ = f.db.Close() })
	if err := f.db.UpsertLocalRegistry(models.NewRegistry(99, "from_cache", true, "", "", "")); err != nil {
		t.Fatalf("failed to upsert registry: %v", err)
	}
	registryID := 99
	_, err := f.PullImage("nginx:latest", &registryID, "")
	if err == nil || !strings.Contains(err.Error(), "from_cache") {
		t.Fatalf("expected from_cache rejection, got: %v", err)
	}
}

func TestFacadePullImage_ResolvesRegistryHostWithRegistryID(t *testing.T) {
	f := NewFacade()
	if err := f.db.Open(t.TempDir()); err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	t.Cleanup(func() { _ = f.db.Close() })
	if err := f.db.UpsertLocalRegistry(models.NewRegistry(98, "quay.io", true, "", "", "")); err != nil {
		t.Fatalf("failed to upsert registry: %v", err)
	}
	registryID := 98
	resolved, err := f.PullImage("skupper/skupper-router", &registryID, "")
	// In unit tests processmanager may not be initialized; resolved value should still be returned.
	if err == nil {
		t.Fatalf("expected process manager init error in unit test")
	}
	if !strings.HasPrefix(resolved, "quay.io/") {
		t.Fatalf("expected resolved image ref with quay.io host, got: %q (err=%v)", resolved, err)
	}
}

func TestFacadeRemoveImage_ValidatesSelector(t *testing.T) {
	f := NewFacade()
	if _, err := f.RemoveImage("   "); err == nil || !strings.Contains(err.Error(), "selector is required") {
		t.Fatalf("expected selector validation error, got: %v", err)
	}
}

func TestFacadePullImage_RegistryNotFound(t *testing.T) {
	f := NewFacade()
	if err := f.db.Open(t.TempDir()); err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	t.Cleanup(func() { _ = f.db.Close() })
	registryID := 123456
	_, err := f.PullImage("nginx:latest", &registryID, "")
	if err == nil {
		t.Fatal("expected error for missing registry")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected not found style error, got: %v", err)
	}
}

func TestFacadeApplyLocalManifest_ProgressStages_DryRun(t *testing.T) {
	f := NewFacade()
	stages := make([]string, 0)
	id, _, err := f.ApplyLocalManifest(testLocalManifestYAML(), "cli", true, func(stage string, _ string) {
		stages = append(stages, strings.TrimSpace(stage))
	})
	if err != nil {
		t.Fatalf("expected dry-run success, got: %v", err)
	}
	if strings.TrimSpace(id) == "" {
		t.Fatalf("expected non-empty deployment id")
	}
	if len(stages) < 2 {
		t.Fatalf("expected at least parsing and done stages, got: %v", stages)
	}
	if stages[0] != DeployStageParsing {
		t.Fatalf("expected first stage %q, got %q (all=%v)", DeployStageParsing, stages[0], stages)
	}
	if stages[len(stages)-1] != DeployStageDone {
		t.Fatalf("expected final stage %q, got %q (all=%v)", DeployStageDone, stages[len(stages)-1], stages)
	}
}

func TestFacadeApplyLocalManifest_ProgressIncludesPersisting_OnFailure(t *testing.T) {
	f := NewFacade()
	stages := make([]string, 0)
	_, _, err := f.ApplyLocalManifest(testLocalManifestYAML(), "cli", false, func(stage string, _ string) {
		stages = append(stages, strings.TrimSpace(stage))
	})
	if err == nil {
		t.Fatalf("expected runtime failure when DB/engine are not fully initialized")
	}
	if len(stages) == 0 {
		t.Fatalf("expected progress stages before failure")
	}
	if stages[0] != DeployStageParsing {
		t.Fatalf("expected first stage %q, got %q (all=%v)", DeployStageParsing, stages[0], stages)
	}
	foundPersisting := false
	for _, stage := range stages {
		if stage == DeployStagePersisting {
			foundPersisting = true
			break
		}
	}
	if !foundPersisting {
		t.Fatalf("expected %q stage before failure, got: %v", DeployStagePersisting, stages)
	}
}

func TestManifestToMicroservice_PropagatesMetadataLabels(t *testing.T) {
	f := NewFacade()
	doc, err := f.ParseAndValidateLocalManifest(testLocalManifestWithLabelsYAML())
	if err != nil {
		t.Fatalf("expected manifest to parse: %v", err)
	}

	ms := manifestToMicroservice(doc, "dep-1", "nginx:latest")
	if got := ms.Labels["team"]; got != "edge" {
		t.Fatalf("expected propagated label team=edge, got %q", got)
	}
	if got := ms.Labels["owner"]; got != "runtime" {
		t.Fatalf("expected propagated label owner=runtime, got %q", got)
	}

	// Ensure microservice labels are decoupled from manifest map mutations.
	doc.Metadata.Labels["team"] = "mutated"
	if got := ms.Labels["team"]; got != "edge" {
		t.Fatalf("expected copied labels map, got team=%q", got)
	}
}
