package runtimeapi

import (
	"errors"
	"strings"
	"testing"

	"github.com/eclipse-iofog/edgelet/internal/dnsresolver"
	"github.com/eclipse-iofog/edgelet/internal/models"
)

func TestFacadeListRuntimeMicroservices_IncludesControlPlaneEntry(t *testing.T) {
	f := NewFacade()
	if err := f.db.Open(t.TempDir()); err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() {
		_ = f.db.Close()
		f.sr.ResetProcessManagerStatus()
	})

	dep := &models.ControlPlaneDeployment{
		ControllerUUID: "cp-uuid-ms",
		Namespace:      "default",
		Name:           "pot",
		ManifestYAML:   "kind: ControlPlane",
		Image:          "datasance/controller:latest",
		State:          "running",
		RuntimeState:   "running",
		ContainerID:    "cid-1",
		DesiredState:   "running",
	}
	if err := f.db.UpsertSystemControlPlane(dep); err != nil {
		t.Fatalf("upsert control plane: %v", err)
	}
	f.sr.UpdateProcessManagerStatus(func(pm *models.ProcessManagerStatus) {
		pm.SetMicroservicesState("cp-uuid-ms", models.MicroserviceStateRunning)
	})

	items := f.ListRuntimeMicroservices()
	if len(items) != 1 {
		t.Fatalf("expected single controlplane entry, got %#v", items)
	}
	item := items[0]
	for key, want := range map[string]string{
		"uuid":        "cp-uuid-ms",
		"name":        "pot",
		"application": "default",
		"source":      "controlplane",
		"type":        "controlplane",
	} {
		if got := item[key]; got != want {
			t.Fatalf("%s: want %q got %v", key, want, got)
		}
	}
}

func TestFacadeGuardControlPlaneMicroserviceMutation_BlocksLifecycle(t *testing.T) {
	f := NewFacade()
	if err := f.db.Open(t.TempDir()); err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = f.db.Close() })

	dep := &models.ControlPlaneDeployment{
		ControllerUUID: "cp-block-1",
		Namespace:      "default",
		Name:           "pot",
		ManifestYAML:   "kind: ControlPlane",
		Image:          "img",
		DesiredState:   "running",
	}
	if err := f.db.UpsertSystemControlPlane(dep); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	for _, op := range []struct {
		name string
		fn   func() (string, error)
	}{
		{"rm", func() (string, error) { return f.RemoveRuntimeMicroservice("cp-block-1") }},
		{"stop", func() (string, error) { return f.StopRuntimeMicroservice("cp-block-1") }},
		{"kill", func() (string, error) { return f.KillRuntimeMicroservice("cp-block-1") }},
		{"start", func() (string, error) { return f.StartRuntimeMicroservice("cp-block-1") }},
		{"restart", func() (string, error) { return f.RestartRuntimeMicroservice("cp-block-1") }},
	} {
		t.Run(op.name, func(t *testing.T) {
			_, err := op.fn()
			if err == nil {
				t.Fatal("expected error")
			}
			var blocked *ErrControlPlaneLifecycleBlocked
			if !errors.As(err, &blocked) {
				t.Fatalf("expected ErrControlPlaneLifecycleBlocked, got %v", err)
			}
			if !strings.Contains(err.Error(), "controlplane delete") {
				t.Fatalf("unexpected message: %v", err)
			}
		})
	}
}

func TestFacadeGetRuntimeMicroservice_ControlPlane(t *testing.T) {
	f := NewFacade()
	if err := f.db.Open(t.TempDir()); err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = f.db.Close() })

	dep := &models.ControlPlaneDeployment{
		ControllerUUID: "cp-get-1",
		Namespace:      "default",
		Name:           "pot",
		ManifestYAML:   "kind: ControlPlane",
		Image:          "img",
		RuntimeState:   "running",
		DesiredState:   "running",
	}
	if err := f.db.UpsertSystemControlPlane(dep); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	item, err := f.GetRuntimeMicroservice("cp-get-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if item["type"] != "controlplane" || item["application"] != "default" || item["name"] != "pot" {
		t.Fatalf("unexpected item: %#v", item)
	}
	raw, ok := item["raw"].(map[string]any)
	if !ok {
		t.Fatalf("expected raw map, got %#v", item["raw"])
	}
	if _, ok := raw["engineInspect"]; !ok {
		t.Fatalf("expected raw.engineInspect, got %#v", raw)
	}
	if _, ok := raw["controlPlaneDeployment"]; ok {
		t.Fatal("ms inspect must not embed controlPlaneDeployment; use controlplane get")
	}
	if raw["inspectSchemaVersion"] != "v1" {
		t.Fatalf("unexpected inspectSchemaVersion: %v", raw["inspectSchemaVersion"])
	}
}

func TestControlPlaneFQDNsFixtureDefaultPot(t *testing.T) {
	// Handoff fixture for IT: namespace=default name=pot
	want := []string{
		"edgelet.controller.svc.bridge.local",
		"controller.default.svc.bridge.local",
		"default.pot.svc.bridge.local",
	}
	got := dnsresolver.ControlPlaneFQDNs("default", "pot")
	if len(got) != len(want) {
		t.Fatalf("want %v got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("want %v got %v", want, got)
		}
	}
}
