package processmanager

import (
	"testing"

	"github.com/eclipse-iofog/edgelet/internal/models"
	"github.com/eclipse-iofog/edgelet/internal/store"
)

func TestIsControlPlaneManagedMicroservice(t *testing.T) {
	pm := &ProcessManager{}
	if err := store.GetInstance().Open(t.TempDir()); err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = store.GetInstance().Close() })

	if err := store.GetInstance().UpsertSystemControlPlane(&models.ControlPlaneDeployment{
		ControllerUUID: "cp-managed-1",
		Namespace:      "default",
		Name:           "pot",
		ManifestYAML:   "kind: ControlPlane",
		DesiredState:   "running",
	}); err != nil {
		t.Fatalf("upsert cp: %v", err)
	}

	ms := models.NewMicroservice("cp-managed-1", "img")
	ms.IsController = true
	if !pm.isControlPlaneManagedMicroservice(ms) {
		t.Fatal("expected controller uuid to be CP-managed")
	}
	other := models.NewMicroservice("other-ms", "img")
	if pm.isControlPlaneManagedMicroservice(other) {
		t.Fatal("expected unrelated ms not CP-managed")
	}
}
