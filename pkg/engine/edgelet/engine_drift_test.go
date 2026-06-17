//go:build linux

package edgelet

import (
	"testing"

	"github.com/eclipse-iofog/edgelet/internal/models"
	"github.com/eclipse-iofog/edgelet/pkg/imageref"
)

func TestMicroserviceImageMatches_DockerHubShortVsQualified(t *testing.T) {
	runtimeImage := "docker.io/emirhandurmus/frame-generator:v1.0.0"
	desiredImage := "emirhandurmus/frame-generator:v1.0.0"
	registry := models.NewRegistry(1, "https://docker.io/", true, "", "", "")
	registryURL, fromCache := imageref.MatchParamsOptional(registryURLFromRegistry(registry))
	if !microserviceImageMatches(runtimeImage, desiredImage, registryURL, fromCache) {
		t.Fatalf("expected %q and %q to match for drift check", runtimeImage, desiredImage)
	}
}

func TestMicroserviceImageMatches_QuayShortVsQualified(t *testing.T) {
	runtimeImage := "quay.io/org/app:v1"
	desiredImage := "org/app:v1"
	registry := models.NewRegistry(3, "quay.io", true, "", "", "")
	registryURL, fromCache := imageref.MatchParamsOptional(registryURLFromRegistry(registry))
	if !microserviceImageMatches(runtimeImage, desiredImage, registryURL, fromCache) {
		t.Fatalf("expected %q and %q to match with quay registry", runtimeImage, desiredImage)
	}
}

func TestMicroserviceImageMatches_QuayDoesNotMatchDockerHubAlias(t *testing.T) {
	runtimeImage := "docker.io/org/app:v1"
	desiredImage := "org/app:v1"
	registry := models.NewRegistry(3, "quay.io", true, "", "", "")
	registryURL, fromCache := imageref.MatchParamsOptional(registryURLFromRegistry(registry))
	if microserviceImageMatches(runtimeImage, desiredImage, registryURL, fromCache) {
		t.Fatalf("quay-bound %q must not match docker.io ref %q", desiredImage, runtimeImage)
	}
}

func TestMicroserviceImageMatches_DifferentTagsMismatch(t *testing.T) {
	runtimeImage := "docker.io/emirhandurmus/frame-generator:v1.0.0"
	desiredImage := "emirhandurmus/frame-generator:v2.0.0"
	registry := models.NewRegistry(1, "https://docker.io/", true, "", "", "")
	registryURL, fromCache := imageref.MatchParamsOptional(registryURLFromRegistry(registry))
	if microserviceImageMatches(runtimeImage, desiredImage, registryURL, fromCache) {
		t.Fatalf("expected tag mismatch between %q and %q", runtimeImage, desiredImage)
	}
}

func TestMatchParamsOptional_FromCacheRegistry(t *testing.T) {
	registry := models.NewRegistry(2, "from_cache", true, "", "", "")
	registryURL, fromCache := imageref.MatchParamsOptional(registryURLFromRegistry(registry))
	if registryURL != "from_cache" || !fromCache {
		t.Fatalf("from_cache registry: url=%q fromCache=%v", registryURL, fromCache)
	}
}
