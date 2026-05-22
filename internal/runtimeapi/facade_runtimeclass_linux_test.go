//go:build linux

package runtimeapi

import (
	"testing"

	"github.com/eclipse-iofog/agent/internal/buildmeta"
	"github.com/eclipse-iofog/agent/internal/config"
	"github.com/eclipse-iofog/agent/internal/models"
	"github.com/eclipse-iofog/agent/pkg/engine/iofog/cri"
)

func TestRuntimeClassApplyDeleteMetadataOnlyAndResolveHandler(t *testing.T) {
	f := NewFacade()
	cfg := config.GetInstance()
	cfg.ContainerEngine = "iofog"
	originalFlavor := buildmeta.Flavor
	buildmeta.Flavor = buildmeta.FlavorFull
	t.Cleanup(func() { buildmeta.Flavor = originalFlavor })

	if err := f.db.Open(t.TempDir()); err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	t.Cleanup(func() { _ = f.db.Close() })

	if _, err := f.ApplyLocalRuntimeClassManifest(`
apiVersion: iofog.org/v3
kind: RuntimeClass
metadata:
  name: edgelet
handler: edgelet
`, false); err != nil {
		t.Fatalf("runtimeclass apply failed: %v", err)
	}

	runtime := "edgelet"
	ms := models.NewMicroservice("u1", "img")
	ms.ApplicationName = "local"
	ms.Runtime = &runtime
	handler, err := cri.GetRuntimeHandler(ms)
	if err != nil {
		t.Fatalf("runtime handler resolution failed after apply: %v", err)
	}
	if handler != "edgelet" {
		t.Fatalf("expected edgelet handler, got %q", handler)
	}

	if err := f.DeleteRuntimeClass("edgelet"); err != nil {
		t.Fatalf("runtimeclass delete failed: %v", err)
	}
	if _, err := cri.GetRuntimeHandler(ms); err == nil {
		t.Fatal("expected runtime handler resolution to fail after runtimeclass delete")
	}
}
