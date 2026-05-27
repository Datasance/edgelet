package statusreporter

import (
	"errors"
	"strings"
	"testing"

	"github.com/datasance/edgelet/internal/buildmeta"
	"github.com/datasance/edgelet/internal/config"
	"github.com/datasance/edgelet/internal/models"
)

func TestGetStatusReport_IncludesAvailableNetworkInterfacesAfterSystemTotalCPU(t *testing.T) {
	report := GetInstance().GetStatusReport()

	totalCPUIdx := strings.Index(report, "System Total CPU")
	availableInterfacesIdx := strings.Index(report, "Available Network Interfaces")
	if totalCPUIdx == -1 {
		t.Fatalf("expected System Total CPU line in report, got:\n%s", report)
	}
	if availableInterfacesIdx == -1 {
		t.Fatalf("expected Available Network Interfaces line in report, got:\n%s", report)
	}
	if availableInterfacesIdx < totalCPUIdx {
		t.Fatalf("expected Available Network Interfaces after System Total CPU, got:\n%s", report)
	}
}

func TestGetStatusReport_IncludesAvailableRuntimes(t *testing.T) {
	cfg := config.GetInstance()
	originalEngine := cfg.ContainerEngine
	cfg.ContainerEngine = "docker"
	t.Cleanup(func() {
		cfg.ContainerEngine = originalEngine
	})

	report := GetInstance().GetStatusReport()
	if !strings.Contains(report, "Available Runtimes          : docker") {
		t.Fatalf("expected available runtimes line in report, got:\n%s", report)
	}
}

func TestGetAvailableRuntimes_DeterministicByEngineAndFlavor(t *testing.T) {
	originalFlavor := buildmeta.Flavor
	originalLister := listRuntimeClassesForStatus
	originalExternal := listExternalRuntimesForStatus
	originalCatalog := listCatalogRuntimesForStatus
	t.Cleanup(func() {
		buildmeta.Flavor = originalFlavor
		listRuntimeClassesForStatus = originalLister
		listExternalRuntimesForStatus = originalExternal
		listCatalogRuntimesForStatus = originalCatalog
	})

	listExternalRuntimesForStatus = func(engineName string) ([]string, error) {
		switch engineName {
		case "docker":
			return []string{"runc", "crun", "runc"}, nil
		case "podman":
			return []string{"crun", "runc", "crun"}, nil
		default:
			return nil, nil
		}
	}
	listRuntimeClassesForStatus = func() ([]*models.LocalRuntimeClass, error) {
		return []*models.LocalRuntimeClass{
			{Name: "edgelet", RuntimeName: "edgelet"},
			{Name: "spin", RuntimeName: "spin"},
		}, nil
	}
	listCatalogRuntimesForStatus = func() []string {
		return []string{"spin", "runc"}
	}

	runtimes := getAvailableRuntimesForEngine("docker", false)
	if strings.Join(runtimes, ",") != "crun,runc" {
		t.Fatalf("expected docker runtime list, got: %v", runtimes)
	}

	runtimes = getAvailableRuntimesForEngine("podman", false)
	if strings.Join(runtimes, ",") != "crun,runc" {
		t.Fatalf("expected podman runtime list, got: %v", runtimes)
	}

	runtimes = getAvailableRuntimesForEngine("edgelet", false)
	if strings.Join(runtimes, ",") != "crun" {
		t.Fatalf("expected baseline iofog runtimes on non-full flavor, got: %v", runtimes)
	}

	buildmeta.Flavor = buildmeta.FlavorFull
	runtimes = getAvailableRuntimesForEngine("edgelet", true)
	if strings.Join(runtimes, ",") != "crun,edgelet,runc,spin" {
		t.Fatalf("expected full flavor iofog runtimes with runtime classes and catalog, got: %v", runtimes)
	}
}

func TestGetAvailableRuntimes_ExternalEngineFallbackOnInfoErrorOrEmpty(t *testing.T) {
	originalExternal := listExternalRuntimesForStatus
	t.Cleanup(func() {
		listExternalRuntimesForStatus = originalExternal
	})

	listExternalRuntimesForStatus = func(engineName string) ([]string, error) {
		if engineName == "docker" {
			return nil, errors.New("daemon unavailable")
		}
		return []string{}, nil
	}

	runtimes := getAvailableRuntimesForEngine("docker", false)
	if strings.Join(runtimes, ",") != "docker" {
		t.Fatalf("expected docker fallback runtime list, got: %v", runtimes)
	}

	runtimes = getAvailableRuntimesForEngine("podman", false)
	if strings.Join(runtimes, ",") != "podman" {
		t.Fatalf("expected podman fallback runtime list, got: %v", runtimes)
	}
}
