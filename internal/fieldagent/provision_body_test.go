package fieldagent

import (
	"strings"
	"testing"

	"github.com/datasance/edgelet/internal/buildmeta"
	"github.com/datasance/edgelet/internal/config"
	"github.com/datasance/edgelet/internal/constants"
)

func TestBuildProvisionRequestBody_LiteDocker(t *testing.T) {
	originalFlavor := buildmeta.Flavor
	buildmeta.Flavor = buildmeta.FlavorLite
	t.Cleanup(func() { buildmeta.Flavor = originalFlavor })

	cfg := config.GetInstance()
	originalEngine := cfg.ContainerEngine
	originalArch := cfg.Arch
	cfg.ContainerEngine = constants.EngineDocker
	cfg.Arch = "amd64"
	t.Cleanup(func() {
		cfg.ContainerEngine = originalEngine
		cfg.Arch = originalArch
	})

	body, err := buildProvisionRequestBody("test-key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if body["key"] != "test-key" {
		t.Fatalf("key=%v", body["key"])
	}
	if body["type"] != 1 {
		t.Fatalf("type=%v", body["type"])
	}
	if body["engine"] != constants.EngineDocker {
		t.Fatalf("engine=%v", body["engine"])
	}
	if body["flavor"] != buildmeta.FlavorLite {
		t.Fatalf("flavor=%v", body["flavor"])
	}
}

func TestBuildProvisionRequestBody_FullIofog(t *testing.T) {
	originalFlavor := buildmeta.Flavor
	buildmeta.Flavor = buildmeta.FlavorFull
	t.Cleanup(func() { buildmeta.Flavor = originalFlavor })

	cfg := config.GetInstance()
	originalEngine := cfg.ContainerEngine
	originalArch := cfg.Arch
	cfg.ContainerEngine = constants.EngineEdgelet
	cfg.Arch = "arm64"
	t.Cleanup(func() {
		cfg.ContainerEngine = originalEngine
		cfg.Arch = originalArch
	})

	body, err := buildProvisionRequestBody("full-key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if body["type"] != 2 {
		t.Fatalf("type=%v", body["type"])
	}
	if body["engine"] != constants.EngineEdgelet {
		t.Fatalf("engine=%v", body["engine"])
	}
	if body["flavor"] != buildmeta.FlavorFull {
		t.Fatalf("flavor=%v", body["flavor"])
	}
}

func TestBuildProvisionRequestBody_RejectsInvalidEngineForFlavor(t *testing.T) {
	originalFlavor := buildmeta.Flavor
	buildmeta.Flavor = buildmeta.FlavorLite
	t.Cleanup(func() { buildmeta.Flavor = originalFlavor })

	cfg := config.GetInstance()
	originalEngine := cfg.ContainerEngine
	cfg.ContainerEngine = constants.EngineEdgelet
	t.Cleanup(func() { cfg.ContainerEngine = originalEngine })

	_, err := buildProvisionRequestBody("bad-key")
	if err == nil {
		t.Fatal("expected error for edgelet engine on lite flavor")
	}
	if !strings.Contains(err.Error(), "provisioning blocked") {
		t.Fatalf("unexpected error: %v", err)
	}
}
