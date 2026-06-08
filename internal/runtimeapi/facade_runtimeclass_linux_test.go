//go:build linux

package runtimeapi

import (
	"testing"

	"github.com/datasance/edgelet/internal/buildmeta"
	"github.com/datasance/edgelet/internal/config"
	"github.com/datasance/edgelet/internal/models"
	"github.com/datasance/edgelet/pkg/engine/edgelet/cri"
)

func TestRuntimeClassApplyDeleteMetadataOnlyAndResolveHandler(t *testing.T) {
	f := NewFacade()
	cfg := config.GetInstance()
	cfg.ContainerEngine = "edgelet"
	embedded := true
	buildmeta.SetHasEmbeddedEngineForTest(&embedded)
	t.Cleanup(func() { buildmeta.SetHasEmbeddedEngineForTest(nil) })

	if err := f.db.Open(t.TempDir()); err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	t.Cleanup(func() { _ = f.db.Close() })

	if _, err := f.ApplyLocalRuntimeClassManifest(`
apiVersion: edgelet.iofog.org/v1
kind: RuntimeClass
metadata:
  name: edgelet-wasmtime
handler: edgelet-wasmtime
`, false); err != nil {
		t.Fatalf("runtimeclass apply failed: %v", err)
	}

	runtime := "edgelet-wasmtime"
	ms := models.NewMicroservice("u1", "img")
	ms.ApplicationName = "edgelet"
	ms.Runtime = &runtime
	handler, err := cri.GetRuntimeHandler(ms)
	if err != nil {
		t.Fatalf("runtime handler resolution failed after apply: %v", err)
	}
	if handler != "edgelet-wasmtime" {
		t.Fatalf("expected edgelet-wasmtime handler, got %q", handler)
	}

	if err := f.DeleteRuntimeClass("edgelet-wasmtime"); err != nil {
		t.Fatalf("runtimeclass delete failed: %v", err)
	}
	if _, err := cri.GetRuntimeHandler(ms); err == nil {
		t.Fatal("expected runtime handler resolution to fail after runtimeclass delete")
	}
}
