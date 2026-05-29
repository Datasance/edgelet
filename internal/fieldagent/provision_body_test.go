package fieldagent

import (
	"strings"
	"testing"

	"github.com/datasance/edgelet/internal/buildmeta"
	"github.com/datasance/edgelet/internal/config"
	"github.com/datasance/edgelet/internal/constants"
)

func TestBuildProvisionRequestBody_DockerEngine(t *testing.T) {
	embedded := true
	buildmeta.SetHasEmbeddedEngineForTest(&embedded)
	t.Cleanup(func() { buildmeta.SetHasEmbeddedEngineForTest(nil) })

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
}

func TestBuildProvisionRequestBody_EdgeletEngine(t *testing.T) {
	embedded := true
	buildmeta.SetHasEmbeddedEngineForTest(&embedded)
	t.Cleanup(func() { buildmeta.SetHasEmbeddedEngineForTest(nil) })

	cfg := config.GetInstance()
	originalEngine := cfg.ContainerEngine
	originalArch := cfg.Arch
	cfg.ContainerEngine = constants.EngineEdgelet
	cfg.Arch = "arm64"
	t.Cleanup(func() {
		cfg.ContainerEngine = originalEngine
		cfg.Arch = originalArch
	})

	body, err := buildProvisionRequestBody("edgelet-key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if body["type"] != 2 {
		t.Fatalf("type=%v", body["type"])
	}
	if body["engine"] != constants.EngineEdgelet {
		t.Fatalf("engine=%v", body["engine"])
	}
}

func TestBuildProvisionRequestBody_RejectsInvalidEngineForPlatform(t *testing.T) {
	embedded := false
	buildmeta.SetHasEmbeddedEngineForTest(&embedded)
	t.Cleanup(func() { buildmeta.SetHasEmbeddedEngineForTest(nil) })

	cfg := config.GetInstance()
	originalEngine := cfg.ContainerEngine
	cfg.ContainerEngine = constants.EngineEdgelet
	t.Cleanup(func() { cfg.ContainerEngine = originalEngine })

	_, err := buildProvisionRequestBody("bad-key")
	if err == nil {
		t.Fatal("expected error for edgelet engine on desktop platform")
	}
	if !strings.Contains(err.Error(), "provisioning blocked") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuildProvisionRequestBody_ArchitectureCodes(t *testing.T) {
	embedded := true
	buildmeta.SetHasEmbeddedEngineForTest(&embedded)
	t.Cleanup(func() { buildmeta.SetHasEmbeddedEngineForTest(nil) })

	cfg := config.GetInstance()
	originalEngine := cfg.ContainerEngine
	originalArch := cfg.Arch
	cfg.ContainerEngine = constants.EngineEdgelet
	t.Cleanup(func() {
		cfg.ContainerEngine = originalEngine
		cfg.Arch = originalArch
	})

	tests := map[string]int{
		"amd64":   1,
		"arm64":   2,
		"riscv64": 3,
		"arm":     4,
	}
	for arch, wantType := range tests {
		cfg.Arch = arch
		body, err := buildProvisionRequestBody("arch-key")
		if err != nil {
			t.Fatalf("arch=%s: %v", arch, err)
		}
		if body["type"] != wantType {
			t.Fatalf("arch=%s type=%v want %d", arch, body["type"], wantType)
		}
	}
}
