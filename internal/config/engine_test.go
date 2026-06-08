package config

import (
	"testing"

	"github.com/eclipse-iofog/edgelet/internal/constants"
)

func TestDefaultContainerEngineURLForEngine(t *testing.T) {
	if got := DefaultContainerEngineURLForEngine(constants.EngineEdgelet); got != constants.EdgeletEngineSocketURL() {
		t.Fatalf("edgelet default URL: got %q", got)
	}
	if got := DefaultContainerEngineURLForEngine(constants.EnginePodman); got != constants.PodmanDefaultDockerURL {
		t.Fatalf("podman default URL: got %q", got)
	}
	if got := DefaultContainerEngineURLForEngine(constants.EngineDocker); got != "unix:///var/run/docker.sock" {
		t.Fatalf("docker default URL: got %q", got)
	}
}

func TestClassifyEngineConfigChange(t *testing.T) {
	cold := ClassifyEngineConfigChange(constants.EngineEdgelet, constants.EngineDocker, "", "", nil)
	if cold != ChangeClassCold {
		t.Fatalf("engine change: got %v want cold", cold)
	}
	warm := ClassifyEngineConfigChange(constants.EngineDocker, constants.EngineDocker,
		"unix:///var/run/docker.sock", "unix:///run/podman/podman.sock", nil)
	if warm != ChangeClassWarm {
		t.Fatalf("url change same engine: got %v want warm", warm)
	}
	hot := ClassifyEngineConfigChange(constants.EngineDocker, constants.EngineDocker,
		"unix:///var/run/docker.sock", "unix:///var/run/docker.sock", nil)
	if hot != ChangeClassHot {
		t.Fatalf("no change: got %v want hot", hot)
	}
}
