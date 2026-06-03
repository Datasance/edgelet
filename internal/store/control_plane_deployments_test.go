package store

import (
	"testing"

	"github.com/datasance/edgelet/internal/models"
)

func TestControlPlaneDeploymentCRUD(t *testing.T) {
	db := openStoreForLocalAPIV3Tests(t)

	dep := &models.ControlPlaneDeployment{
		ControllerUUID: "cp-uuid-1",
		Namespace:      "default",
		Name:           "pot",
		ManifestYAML:   "kind: ControlPlane",
		Image:          "datasance/controller:latest",
		State:          "running",
		ContainerID:    "cid-cp-1",
		Generation:     2,
	}
	if err := db.UpsertControlPlaneDeployment(dep); err != nil {
		t.Fatalf("upsert control plane deployment: %v", err)
	}

	got, found, err := db.GetControlPlaneDeployment()
	if err != nil {
		t.Fatalf("get control plane deployment: %v", err)
	}
	if !found {
		t.Fatal("expected deployment to exist")
	}
	if got.ControllerUUID != "cp-uuid-1" {
		t.Fatalf("unexpected controller_uuid: %q", got.ControllerUUID)
	}
	if got.Namespace != "default" {
		t.Fatalf("unexpected namespace: %q", got.Namespace)
	}
	if got.Name != "pot" {
		t.Fatalf("unexpected name: %q", got.Name)
	}
	if got.Image != "datasance/controller:latest" {
		t.Fatalf("unexpected image: %q", got.Image)
	}
	if got.ContainerID != "cid-cp-1" {
		t.Fatalf("unexpected container_id: %q", got.ContainerID)
	}
	if got.DesiredState != "running" {
		t.Fatalf("expected desired_state running, got %q", got.DesiredState)
	}
	if got.Generation != 2 {
		t.Fatalf("expected generation=2, got %d", got.Generation)
	}

	dep.Image = "datasance/controller:v2"
	dep.Generation = 3
	if err := db.UpsertControlPlaneDeployment(dep); err != nil {
		t.Fatalf("upsert control plane deployment patch: %v", err)
	}

	got, found, err = db.GetControlPlaneDeployment()
	if err != nil {
		t.Fatalf("get control plane deployment after patch: %v", err)
	}
	if !found || got.Image != "datasance/controller:v2" || got.Generation != 3 {
		t.Fatalf("unexpected row after patch: found=%v image=%q generation=%d", found, got.Image, got.Generation)
	}

	if err := db.DeleteControlPlaneDeployment(); err != nil {
		t.Fatalf("delete control plane deployment: %v", err)
	}

	_, found, err = db.GetControlPlaneDeployment()
	if err != nil {
		t.Fatalf("get control plane deployment after delete: %v", err)
	}
	if found {
		t.Fatal("expected no deployment after delete")
	}
}

func TestControlPlaneDeploymentSingletonConstraint(t *testing.T) {
	db := openStoreForLocalAPIV3Tests(t)

	dep := &models.ControlPlaneDeployment{
		ControllerUUID: "cp-uuid-1",
		Name:           "pot",
		ManifestYAML:   "kind: ControlPlane",
	}
	if err := db.UpsertControlPlaneDeployment(dep); err != nil {
		t.Fatalf("upsert control plane deployment: %v", err)
	}

	_, err := db.Conn().Exec(`INSERT INTO control_plane_deployments (
		id, controller_uuid, namespace, name, manifest_yaml
	) VALUES (2, 'other-uuid', 'default', 'other', 'kind: ControlPlane')`)
	if err == nil {
		t.Fatal("expected CHECK constraint to reject second control_plane_deployments row")
	}
}

func TestControlPlaneDeploymentUpsertValidation(t *testing.T) {
	db := openStoreForLocalAPIV3Tests(t)

	if err := db.UpsertControlPlaneDeployment(nil); err == nil {
		t.Fatal("expected error for nil deployment")
	}
	if err := db.UpsertControlPlaneDeployment(&models.ControlPlaneDeployment{
		Name:         "pot",
		ManifestYAML: "kind: ControlPlane",
	}); err == nil {
		t.Fatal("expected error for missing controller_uuid")
	}
}
