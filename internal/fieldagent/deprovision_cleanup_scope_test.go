package fieldagent

import (
	"fmt"
	"testing"

	"github.com/datasance/edgelet/internal/buildmeta"
	"github.com/datasance/edgelet/internal/constants"
	"github.com/datasance/edgelet/internal/models"
	"github.com/datasance/edgelet/internal/store"
)

func TestClearSQLiteCacheTablesOnDeprovision_AllClearsLocalRows(t *testing.T) {
	db := store.GetInstance()
	_ = db.Close()
	if err := db.Open(t.TempDir()); err != nil {
		t.Fatalf("failed to open sqlite DB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	local := &models.LocalDeployedMicroservice{
		LocalUUID:        "local-1",
		ApplicationName:  "edgelet",
		MicroserviceName: "router",
		SourceName:       "local-cli",
		ManifestYAML:     "kind: Microservice",
		ImageName:        "nginx:latest",
		State:            "running",
	}
	if err := db.UpsertLocalDeployedMicroservice(local); err != nil {
		t.Fatalf("failed to insert local deployment: %v", err)
	}
	if err := db.SaveLocalContainerState("local-1", "cid-1", "sandbox-1"); err != nil {
		t.Fatalf("failed to insert local container state: %v", err)
	}

	GetInstance().clearSQLiteCacheTablesOnDeprovision(false)

	locals, err := db.ListLocalDeployedMicroservices()
	if err != nil {
		t.Fatalf("failed listing locals: %v", err)
	}
	if len(locals) != 0 {
		t.Fatalf("expected local deployments cleared, got %d", len(locals))
	}
	state, err := db.GetLocalContainerState("local-1")
	if err != nil {
		t.Fatalf("failed reading local container state: %v", err)
	}
	if state != nil {
		t.Fatalf("expected local container state cleared, got %#v", state)
	}
}

func TestClearSQLiteCacheTablesOnDeprovision_LocalScopePreservesLocalRows(t *testing.T) {
	db := store.GetInstance()
	_ = db.Close()
	if err := db.Open(t.TempDir()); err != nil {
		t.Fatalf("failed to open sqlite DB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	local := &models.LocalDeployedMicroservice{
		LocalUUID:        "local-2",
		ApplicationName:  "edgelet",
		MicroserviceName: "nodered",
		SourceName:       "local-cli",
		ManifestYAML:     "kind: Microservice",
		ImageName:        "nodered/node-red:latest",
		State:            "running",
	}
	if err := db.UpsertLocalDeployedMicroservice(local); err != nil {
		t.Fatalf("failed to insert local deployment: %v", err)
	}
	if err := db.SaveLocalContainerState("local-2", "cid-2", "sandbox-2"); err != nil {
		t.Fatalf("failed to insert local container state: %v", err)
	}

	GetInstance().clearSQLiteCacheTablesOnDeprovision(true)

	locals, err := db.ListLocalDeployedMicroservices()
	if err != nil {
		t.Fatalf("failed listing locals: %v", err)
	}
	if len(locals) != 1 {
		t.Fatalf("expected local deployment preserved, got %d", len(locals))
	}
	state, err := db.GetLocalContainerState("local-2")
	if err != nil {
		t.Fatalf("failed reading local container state: %v", err)
	}
	if state == nil {
		t.Fatalf("expected local container state preserved")
	}
}

func TestClearVolumeMountsOnDeprovision_LocalScopeSkipsClear(t *testing.T) {
	allCalled := false
	localCalled := false
	GetInstance().clearVolumeMountsOnDeprovision(true, func() error {
		allCalled = true
		return nil
	}, func() error {
		localCalled = true
		return nil
	})
	if allCalled {
		t.Fatalf("expected all-scope clear function not to be called for preserveLocal=true")
	}
	if !localCalled {
		t.Fatalf("expected local-scope clear function to be called for preserveLocal=true")
	}
}

func TestClearVolumeMountsOnDeprovision_AllScopeInvokesClear(t *testing.T) {
	allCalled := false
	localCalled := false
	GetInstance().clearVolumeMountsOnDeprovision(false, func() error {
		allCalled = true
		return nil
	}, func() error {
		localCalled = true
		return nil
	})
	if !allCalled {
		t.Fatalf("expected all-scope clear function to be called for preserveLocal=false")
	}
	if localCalled {
		t.Fatalf("expected local-scope clear function not to be called for preserveLocal=false")
	}
}

func TestClearVolumeMountsOnDeprovision_AllScopeRecoversPanic(t *testing.T) {
	GetInstance().clearVolumeMountsOnDeprovision(false, func() error {
		panic("boom")
	}, func() error {
		return nil
	})
}

func TestClearVolumeMountsOnDeprovision_AllScopeHandlesError(t *testing.T) {
	called := false
	GetInstance().clearVolumeMountsOnDeprovision(false, func() error {
		called = true
		return fmt.Errorf("clear failed")
	}, func() error {
		return nil
	})
	if !called {
		t.Fatalf("expected clear function to be called")
	}
}

func TestClearLiteRuntimeArtifactsOnDeprovision_LocalScopeSkipsPrune(t *testing.T) {
	originalFlavor := buildmeta.Flavor
	buildmeta.Flavor = buildmeta.FlavorLite
	t.Cleanup(func() { buildmeta.Flavor = originalFlavor })
	GetInstance().config.ContainerEngine = constants.EngineDocker

	containersCalled := false
	volumesCalled := false
	GetInstance().clearLiteRuntimeArtifactsOnDeprovision(true, func() error {
		containersCalled = true
		return nil
	}, func() error {
		volumesCalled = true
		return nil
	})
	if containersCalled || volumesCalled {
		t.Fatalf("expected prune steps to be skipped for preserveLocal=true")
	}
}

func TestClearLiteRuntimeArtifactsOnDeprovision_FullFlavorSkipsPrune(t *testing.T) {
	originalFlavor := buildmeta.Flavor
	buildmeta.Flavor = buildmeta.FlavorFull
	t.Cleanup(func() { buildmeta.Flavor = originalFlavor })
	GetInstance().config.ContainerEngine = constants.EngineDocker

	containersCalled := false
	volumesCalled := false
	GetInstance().clearLiteRuntimeArtifactsOnDeprovision(false, func() error {
		containersCalled = true
		return nil
	}, func() error {
		volumesCalled = true
		return nil
	})
	if containersCalled || volumesCalled {
		t.Fatalf("expected prune steps to be skipped for full flavor")
	}
}

func TestClearLiteRuntimeArtifactsOnDeprovision_OnlyLiteDockerPodman(t *testing.T) {
	originalFlavor := buildmeta.Flavor
	buildmeta.Flavor = buildmeta.FlavorLite
	t.Cleanup(func() { buildmeta.Flavor = originalFlavor })

	GetInstance().config.ContainerEngine = constants.EngineEdgelet
	called := false
	GetInstance().clearLiteRuntimeArtifactsOnDeprovision(false, func() error {
		called = true
		return nil
	}, nil)
	if called {
		t.Fatalf("expected prune steps skipped for non-docker/podman engine")
	}
}

func TestClearLiteRuntimeArtifactsOnDeprovision_OrderAndResilience(t *testing.T) {
	originalFlavor := buildmeta.Flavor
	buildmeta.Flavor = buildmeta.FlavorLite
	t.Cleanup(func() { buildmeta.Flavor = originalFlavor })
	GetInstance().config.ContainerEngine = constants.EnginePodman

	order := make([]string, 0, 2)
	GetInstance().clearLiteRuntimeArtifactsOnDeprovision(false, func() error {
		order = append(order, "containers")
		return fmt.Errorf("container prune failed")
	}, func() error {
		order = append(order, "volumes")
		return nil
	})
	if len(order) != 2 || order[0] != "containers" || order[1] != "volumes" {
		t.Fatalf("expected prune order containers->volumes, got %v", order)
	}
}
