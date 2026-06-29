package fieldagent

import (
	"strings"
	"testing"

	"github.com/eclipse-iofog/edgelet/internal/config"
	"github.com/eclipse-iofog/edgelet/internal/controlplane"
	"github.com/eclipse-iofog/edgelet/internal/models"
	"github.com/eclipse-iofog/edgelet/internal/store"
)

func TestBuildControllerRegisterBody(t *testing.T) {
	cfg := config.GetInstance()
	cfg.Arch = "amd64"

	cp := &models.ControlPlaneDeployment{
		ControllerUUID: "cp-reg-1",
		Namespace:      "default",
		Name:           "pot",
		ManifestYAML: `apiVersion: edgelet.iofog.org/v1
kind: ControlPlane
metadata:
  namespace: default
  name: pot
spec:
  controller:
    image: ghcr.io/datasance/controller:3.8.0-beta.0
    registry: 2
  auth:
    mode: embedded
    bootstrap:
      username: admin
      password: AdminPass123!
`,
	}

	body, err := buildControllerRegisterBody(cfg, cp)
	if err != nil {
		t.Fatalf("build body: %v", err)
	}
	if body["uuid"] != "cp-reg-1" {
		t.Fatalf("uuid: got %#v", body["uuid"])
	}
	if body["name"] != "controller" {
		t.Fatalf("name: got %#v", body["name"])
	}
	if schedule, ok := body["schedule"].(int); !ok || schedule != 0 {
		t.Fatalf("schedule: got %#v", body["schedule"])
	}
	images, ok := body["images"].([]map[string]any)
	if !ok || len(images) != 1 {
		t.Fatalf("images: got %#v", body["images"])
	}
	if images[0]["archId"] != 1 {
		t.Fatalf("archId: got %#v", images[0]["archId"])
	}
	env, ok := body["env"].([]map[string]any)
	if !ok || len(env) == 0 {
		t.Fatalf("env: got %#v", body["env"])
	}
	for _, item := range env {
		if _, hasKey := item["key"]; !hasKey {
			t.Fatalf("env item missing key: %#v", item)
		}
		if _, hasValue := item["value"]; !hasValue {
			t.Fatalf("env item missing value: %#v", item)
		}
	}

	ports, ok := body["ports"].([]map[string]any)
	if !ok || len(ports) != 2 {
		t.Fatalf("ports: got %#v", body["ports"])
	}
	for _, port := range ports {
		if _, ok := port["internal"]; !ok {
			t.Fatalf("port missing internal: %#v", port)
		}
		if _, ok := port["external"]; !ok {
			t.Fatalf("port missing external: %#v", port)
		}
		if protocol, ok := port["protocol"].(string); !ok || protocol == "" {
			t.Fatalf("port missing protocol: %#v", port)
		}
		if _, ok := port["portExternal"]; ok {
			t.Fatalf("port must not use portExternal wire key: %#v", port)
		}
	}
	if ports[0]["internal"] != controlplane.DefaultContainerAPIPort {
		t.Fatalf("api internal port: got %#v want %d", ports[0]["internal"], controlplane.DefaultContainerAPIPort)
	}
	if ports[0]["external"] != controlplane.HostAPIPort {
		t.Fatalf("api external port: got %#v want %d", ports[0]["external"], controlplane.HostAPIPort)
	}

	volumeMappings, ok := body["volumeMappings"].([]map[string]any)
	if !ok || len(volumeMappings) < 2 {
		t.Fatalf("volumeMappings: got %#v", body["volumeMappings"])
	}
	for _, vm := range volumeMappings {
		for _, key := range []string{"hostDestination", "containerDestination", "accessMode", "type"} {
			if _, ok := vm[key]; !ok {
				t.Fatalf("volume mapping missing %s: %#v", key, vm)
			}
		}
	}

	capAdd, ok := body["capAdd"].([]string)
	if !ok || len(capAdd) == 0 {
		t.Fatalf("capAdd: got %#v", body["capAdd"])
	}
	hasNetRaw := false
	for _, cap := range capAdd {
		if strings.EqualFold(cap, "NET_RAW") {
			hasNetRaw = true
		}
	}
	if !hasNetRaw {
		t.Fatalf("capAdd must include NET_RAW, got %#v", capAdd)
	}
}

func TestControllerRegisterState(t *testing.T) {
	state := newControllerRegisterState()
	if state.isSucceeded("uuid-1") {
		t.Fatal("expected not succeeded initially")
	}
	state.markSucceeded("uuid-1")
	if !state.isSucceeded("uuid-1") {
		t.Fatal("expected succeeded after mark")
	}
	state.reset()
	if state.isSucceeded("uuid-1") {
		t.Fatal("expected reset to clear succeeded state")
	}
}

func TestParseMicroservice_IsController(t *testing.T) {
	ms, err := parseMicroservice(map[string]any{
		"uuid":         "ms-1",
		"imageId":      "alpine:3.19",
		"isController": true,
		"isSystem":     true,
	})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !ms.IsController || !ms.IsSystem {
		t.Fatalf("flags: controller=%v system=%v", ms.IsController, ms.IsSystem)
	}
}

func TestControllerRegisterState_InitialRebuildSkipped(t *testing.T) {
	state := newControllerRegisterState()
	const uuid = "cp-state-1"

	state.markSucceeded(uuid)
	if !state.isSucceeded(uuid) {
		t.Fatal("expected register succeeded marker")
	}
	if state.isInitialRebuildSkipped(uuid) {
		t.Fatal("expected initial rebuild not skipped yet")
	}

	state.markInitialRebuildSkipped(uuid)
	if !state.isInitialRebuildSkipped(uuid) {
		t.Fatal("expected initial rebuild skipped marker")
	}

	state.reset()
	if state.isSucceeded(uuid) {
		t.Fatal("expected succeeded cleared on reset")
	}
	if state.isInitialRebuildSkipped(uuid) {
		t.Fatal("expected initial rebuild skip cleared on reset")
	}
}

func TestHydrateControllerRegisterStateFromDB(t *testing.T) {
	openFieldAgentTestDB(t)

	const uuid = "cp-hydrate"
	if err := store.GetInstance().UpsertSystemControlPlane(&models.ControlPlaneDeployment{
		ControllerUUID:        uuid,
		Name:                  "pot",
		ManifestYAML:          minimalReconcileManifestYAML(),
		ControllerRegistered:  true,
		InitialRebuildSkipped: true,
	}); err != nil {
		t.Fatalf("upsert control plane: %v", err)
	}

	fa := &FieldAgent{controllerRegister: newControllerRegisterState()}
	fa.hydrateControllerRegisterState()

	if !fa.controllerRegister.isSucceeded(uuid) {
		t.Fatal("expected register state hydrated from sqlite")
	}
	if !fa.controllerRegister.isInitialRebuildSkipped(uuid) {
		t.Fatal("expected initial rebuild skip hydrated from sqlite")
	}
}
