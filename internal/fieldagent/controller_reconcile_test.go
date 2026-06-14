package fieldagent

import (
	"testing"

	"github.com/eclipse-iofog/edgelet/internal/models"
	"github.com/eclipse-iofog/edgelet/internal/store"
)

func openFieldAgentTestDB(t *testing.T) {
	t.Helper()
	db := store.GetInstance()
	_ = db.Close()
	if err := db.Open(t.TempDir()); err != nil {
		t.Fatalf("open sqlite DB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
}

func minimalControlPlaneForReconcileTest(uuid string) *models.ControlPlaneDeployment {
	return &models.ControlPlaneDeployment{
		ControllerUUID:     uuid,
		Namespace:          "default",
		Name:               "pot",
		ManifestYAML:       minimalReconcileManifestYAML(),
		DesiredState:       "running",
		Generation:         1,
		ObservedGeneration: 1,
		RuntimeState:       "running",
		Image:              "ghcr.io/datasance/controller:3.8.0-beta.0",
	}
}

func minimalReconcileManifestYAML() string {
	return `apiVersion: edgelet.iofog.org/v1
kind: ControlPlane
metadata:
  name: pot
  namespace: default
spec:
  controller:
    image: ghcr.io/datasance/controller:3.8.0-beta.0
  auth:
    mode: embedded
    bootstrap:
      username: admin
      password: AdminPass123!
`
}

func TestReconcileControllerMicroservice_SkipsInitialRebuildAfterRegister(t *testing.T) {
	openFieldAgentTestDB(t)

	const uuid = "cp-reconcile-skip"
	cp := minimalControlPlaneForReconcileTest(uuid)
	if err := store.GetInstance().UpsertSystemControlPlane(cp); err != nil {
		t.Fatalf("upsert control plane: %v", err)
	}

	fa := &FieldAgent{
		controllerRegister: newControllerRegisterState(),
	}
	fa.controllerRegister.markSucceeded(uuid)

	msList := []*models.Microservice{{
		MicroserviceUUID: uuid,
		IsController:     true,
		Rebuild:          true,
		ImageName:        cp.Image,
	}}
	fa.reconcileControllerMicroservice(msList)

	got, found, err := store.GetInstance().GetSystemControlPlane()
	if err != nil || !found {
		t.Fatalf("get control plane: found=%v err=%v", found, err)
	}
	if got.Generation != 1 {
		t.Fatalf("expected generation unchanged at 1, got %d", got.Generation)
	}
	if !fa.controllerRegister.isInitialRebuildSkipped(uuid) {
		t.Fatal("expected initial rebuild skip marker")
	}

	got, found, err = store.GetInstance().GetSystemControlPlane()
	if err != nil || !found {
		t.Fatalf("get control plane after skip: found=%v err=%v", found, err)
	}
	if !got.InitialRebuildSkipped {
		t.Fatal("expected initial_rebuild_skipped persisted in sqlite")
	}
	if msList[0].Rebuild {
		t.Fatal("expected rebuild flag cleared on microservice")
	}
}

func TestReconcileControllerMicroservice_HonorsLaterRebuild(t *testing.T) {
	openFieldAgentTestDB(t)

	const uuid = "cp-reconcile-pull"
	cp := minimalControlPlaneForReconcileTest(uuid)
	if err := store.GetInstance().UpsertSystemControlPlane(cp); err != nil {
		t.Fatalf("upsert control plane: %v", err)
	}

	fa := &FieldAgent{
		controllerRegister: newControllerRegisterState(),
	}
	fa.controllerRegister.markSucceeded(uuid)
	fa.controllerRegister.markInitialRebuildSkipped(uuid)

	msList := []*models.Microservice{{
		MicroserviceUUID: uuid,
		IsController:     true,
		Rebuild:          true,
		ImageName:        cp.Image,
	}}
	fa.reconcileControllerMicroservice(msList)

	got, found, err := store.GetInstance().GetSystemControlPlane()
	if err != nil || !found {
		t.Fatalf("get control plane: found=%v err=%v", found, err)
	}
	if got.Generation != 2 {
		t.Fatalf("expected generation 2, got %d", got.Generation)
	}
}

func TestReconcileControllerMicroservice_IgnoresEquivalentDockerHubImageRef(t *testing.T) {
	openFieldAgentTestDB(t)
	if err := store.GetInstance().EnsureDefaultLocalRegistries(); err != nil {
		t.Fatalf("seed registries: %v", err)
	}

	const uuid = "cp-reconcile-image-alias"
	cp := &models.ControlPlaneDeployment{
		ControllerUUID:     uuid,
		Namespace:          "default",
		Name:               "pot",
		ManifestYAML: `apiVersion: edgelet.iofog.org/v1
kind: ControlPlane
metadata:
  name: pot
  namespace: default
spec:
  controller:
    image: emirhandurmus/controller:3.8.0-beta.1
    registry: 1
  auth:
    mode: embedded
    bootstrap:
      username: admin
      password: AdminPass123!
`,
		DesiredState:       "running",
		Generation:         1,
		ObservedGeneration: 1,
		RuntimeState:       "running",
		Image:              "docker.io/emirhandurmus/controller:3.8.0-beta.1",
	}
	if err := store.GetInstance().UpsertSystemControlPlane(cp); err != nil {
		t.Fatalf("upsert control plane: %v", err)
	}

	fa := &FieldAgent{}
	msList := []*models.Microservice{{
		MicroserviceUUID: uuid,
		IsController:     true,
		ImageName:        "emirhandurmus/controller:3.8.0-beta.1",
		RegistryID:       1,
	}}
	fa.reconcileControllerMicroservice(msList)

	got, found, err := store.GetInstance().GetSystemControlPlane()
	if err != nil || !found {
		t.Fatalf("get control plane: found=%v err=%v", found, err)
	}
	if got.Generation != 1 {
		t.Fatalf("expected generation unchanged at 1, got %d", got.Generation)
	}
}

func TestReconcileControllerMicroservice_BumpsGenerationOnImageChange(t *testing.T) {
	openFieldAgentTestDB(t)
	if err := store.GetInstance().EnsureDefaultLocalRegistries(); err != nil {
		t.Fatalf("seed registries: %v", err)
	}

	const uuid = "cp-reconcile-image-change"
	cp := &models.ControlPlaneDeployment{
		ControllerUUID:     uuid,
		Namespace:          "default",
		Name:               "pot",
		ManifestYAML: `apiVersion: edgelet.iofog.org/v1
kind: ControlPlane
metadata:
  name: pot
  namespace: default
spec:
  controller:
    image: emirhandurmus/controller:3.8.0-beta.1
    registry: 1
  auth:
    mode: embedded
    bootstrap:
      username: admin
      password: AdminPass123!
`,
		DesiredState:       "running",
		Generation:         1,
		ObservedGeneration: 1,
		RuntimeState:       "running",
		Image:              "docker.io/emirhandurmus/controller:3.8.0-beta.1",
	}
	if err := store.GetInstance().UpsertSystemControlPlane(cp); err != nil {
		t.Fatalf("upsert control plane: %v", err)
	}

	fa := &FieldAgent{}
	msList := []*models.Microservice{{
		MicroserviceUUID: uuid,
		IsController:     true,
		ImageName:        "emirhandurmus/controller:3.8.0-beta.2",
		RegistryID:       1,
	}}
	fa.reconcileControllerMicroservice(msList)

	got, found, err := store.GetInstance().GetSystemControlPlane()
	if err != nil || !found {
		t.Fatalf("get control plane: found=%v err=%v", found, err)
	}
	if got.Generation != 2 {
		t.Fatalf("expected generation 2, got %d", got.Generation)
	}
	if got.Image != "emirhandurmus/controller:3.8.0-beta.2" {
		t.Fatalf("expected merged pot image ref, got %q", got.Image)
	}
}

func TestReconcileControllerMicroservice_BumpsGenerationOnRegistryDrift(t *testing.T) {
	openFieldAgentTestDB(t)
	if err := store.GetInstance().EnsureDefaultLocalRegistries(); err != nil {
		t.Fatalf("seed registries: %v", err)
	}

	const uuid = "cp-reconcile-registry-drift"
	cp := &models.ControlPlaneDeployment{
		ControllerUUID:     uuid,
		Namespace:          "default",
		Name:               "pot",
		ManifestYAML: `apiVersion: edgelet.iofog.org/v1
kind: ControlPlane
metadata:
  name: pot
  namespace: default
spec:
  controller:
    image: emirhandurmus/controller:3.8.0-beta.1
    registry: 2
  auth:
    mode: embedded
    bootstrap:
      username: admin
      password: AdminPass123!
`,
		DesiredState:       "running",
		Generation:         1,
		ObservedGeneration: 1,
		RuntimeState:       "running",
		Image:              "emirhandurmus/controller:3.8.0-beta.1",
	}
	if err := store.GetInstance().UpsertSystemControlPlane(cp); err != nil {
		t.Fatalf("upsert control plane: %v", err)
	}

	fa := &FieldAgent{}
	msList := []*models.Microservice{{
		MicroserviceUUID: uuid,
		IsController:     true,
		ImageName:        "emirhandurmus/controller:3.8.0-beta.1",
		RegistryID:       1,
	}}
	fa.reconcileControllerMicroservice(msList)

	got, found, err := store.GetInstance().GetSystemControlPlane()
	if err != nil || !found {
		t.Fatalf("get control plane: found=%v err=%v", found, err)
	}
	if got.Generation != 2 {
		t.Fatalf("expected generation 2, got %d", got.Generation)
	}
}

func TestControllerRegistryDrift(t *testing.T) {
	regTwo := 2
	doc := &models.ControlPlaneManifest{
		Spec: models.ControlPlaneManifestSpec{
			Controller: struct {
				Image      string `yaml:"image" json:"image"`
				Registry   *int   `yaml:"registry,omitempty" json:"registry,omitempty"`
				Port       *int   `yaml:"port,omitempty" json:"port,omitempty"`
				PublicURL  string `yaml:"publicUrl,omitempty" json:"publicUrl,omitempty"`
				TrustProxy *bool  `yaml:"trustProxy,omitempty" json:"trustProxy,omitempty"`
			}{
				Registry: &regTwo,
			},
		},
	}
	if !controllerRegistryDrift(1, doc) {
		t.Fatal("expected registry drift when pot registry differs from manifest")
	}
	if controllerRegistryDrift(2, doc) {
		t.Fatal("expected no registry drift when registries match")
	}
	if !controllerRegistryDrift(1, &models.ControlPlaneManifest{}) {
		t.Fatal("expected registry drift when manifest registry is unset")
	}
}
