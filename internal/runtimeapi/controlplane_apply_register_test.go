package runtimeapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/eclipse-iofog/edgelet/internal/config"
	"github.com/eclipse-iofog/edgelet/internal/fieldagent"
	"github.com/eclipse-iofog/edgelet/internal/models"
	"github.com/eclipse-iofog/edgelet/internal/processmanager"
	"github.com/eclipse-iofog/edgelet/internal/store"
)

func TestApplyControlPlaneManifest_PatchUpsertsControllerRegister(t *testing.T) {
	const uuid = "cp-patch-register"
	var registerCalls atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/agent/controller/register") {
			registerCalls.Add(1)
			_, _ = w.Write([]byte(`{"uuid":"` + uuid + `"}`))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	f := NewFacade()
	if err := f.db.Open(t.TempDir()); err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() {
		_ = f.db.Close()
		f.sr.ResetProcessManagerStatus()
		processmanager.ResetProcessManagerEngineForTest()
		fieldagent.ResetControllerRegisterStateForTest()
	})

	eng := &cpRestartTestEngine{
		containerID:      "cid-patch",
		microserviceUUID: uuid,
	}
	processmanager.ConfigureControlPlaneRestartForTest(eng, "edgelet", func(item *models.ControlPlaneDeployment, _ bool, now int64) error {
		item.RuntimeState = "running"
		item.State = "running"
		item.ContainerID = "cid-patch"
		item.ObservedGeneration = item.Generation
		item.LastTransitionAt = now
		item.LastError = ""
		return store.GetInstance().UpsertSystemControlPlane(item)
	})

	cfg := config.GetInstance()
	origURL := cfg.ControllerURL
	cfg.ControllerURL = srv.URL
	t.Cleanup(func() { cfg.ControllerURL = origURL })

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	fieldagent.GetInstance().SetProcessManager(processmanager.GetInstance())
	fieldagent.MarkControllerRegisterSucceededForTest(uuid)
	fieldagent.ConfigureRegisterTestRuntime(ctx, true)
	fieldagent.ConfigureRegisterTestClient(srv.URL, srv.Client())

	existing := &models.ControlPlaneDeployment{
		ControllerUUID:       uuid,
		Namespace:            "default",
		Name:                 "pot",
		ManifestYAML:         testControlPlaneManifestYAML(),
		Image:                "ghcr.io/datasance/controller:3.8.0-beta.0",
		DesiredState:         "running",
		RuntimeState:         "running",
		ContainerID:          "cid-patch",
		Generation:           1,
		ObservedGeneration:   1,
		ControllerRegistered: true,
	}
	if err := f.db.UpsertSystemControlPlane(existing); err != nil {
		t.Fatalf("upsert existing: %v", err)
	}

	patchManifest := strings.Replace(
		testControlPlaneManifestYAML(),
		"ghcr.io/datasance/controller:3.8.0-beta.0",
		"ghcr.io/datasance/controller:3.8.0-beta.1",
		1,
	)
	result, err := f.ApplyControlPlaneManifest(patchManifest, "controlplane.yaml", false, nil)
	if err != nil {
		t.Fatalf("ApplyControlPlaneManifest patch: %v", err)
	}
	if result.Mode != "patch" {
		t.Fatalf("mode: got %q want patch", result.Mode)
	}
	if got := registerCalls.Load(); got != 1 {
		t.Fatalf("expected one controller/register upsert on patch, got %d", got)
	}
}
