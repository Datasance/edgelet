//go:build linux

package cri

import (
	"testing"

	"github.com/datasance/edgelet/internal/models"
)

func TestContainerConfigFromMicroserviceAppliesResourceLimits(t *testing.T) {
	mem := int64(64 * 1024 * 1024)
	cpus := "0-1"
	ms := &models.Microservice{
		MicroserviceUUID: "ms-1",
		MicroserviceName: "limits",
		ImageName:        "docker.io/library/alpine:3.19",
		MemoryLimit:      &mem,
		CPUSetCpus:       &cpus,
	}

	cfg, err := ContainerConfigFromMicroservice(ms, "host", nil, "0.log", "", "", "sandbox-1", "node-1")
	if err != nil {
		t.Fatalf("ContainerConfigFromMicroservice: %v", err)
	}
	if cfg.Linux == nil || cfg.Linux.Resources == nil {
		t.Fatal("expected linux resources block")
	}
	if cfg.Linux.Resources.MemoryLimitInBytes != mem {
		t.Fatalf("memory limit = %d want %d", cfg.Linux.Resources.MemoryLimitInBytes, mem)
	}
	if cfg.Linux.Resources.CpusetCpus != cpus {
		t.Fatalf("cpuset = %q want %q", cfg.Linux.Resources.CpusetCpus, cpus)
	}
}

func TestContainerConfigFromMicroserviceOmitsZeroMemoryLimit(t *testing.T) {
	zero := int64(0)
	ms := &models.Microservice{
		MicroserviceUUID: "ms-2",
		MicroserviceName: "no-limit",
		ImageName:        "docker.io/library/alpine:3.19",
		MemoryLimit:      &zero,
	}

	cfg, err := ContainerConfigFromMicroservice(ms, "host", nil, "0.log", "", "", "sandbox-2", "node-2")
	if err != nil {
		t.Fatalf("ContainerConfigFromMicroservice: %v", err)
	}
	if cfg.Linux.Resources.MemoryLimitInBytes != 0 {
		t.Fatalf("expected zero memory limit, got %d", cfg.Linux.Resources.MemoryLimitInBytes)
	}
}
