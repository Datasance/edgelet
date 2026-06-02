package config

import (
	"testing"

	"github.com/datasance/edgelet/internal/constants"
)

func TestDefaultDockerURLForEngine(t *testing.T) {
	if got := DefaultDockerURLForEngine(constants.EngineEdgelet); got != constants.EdgeletEngineDockerURL() {
		t.Fatalf("edgelet default = %q", got)
	}
	if got := DefaultDockerURLForEngine(constants.EnginePodman); got != constants.PodmanDefaultDockerURL {
		t.Fatalf("podman default = %q", got)
	}
	if got := DefaultDockerURLForEngine(constants.EngineDocker); got != "unix:///var/run/docker.sock" {
		t.Fatalf("docker default = %q", got)
	}
}

func TestClassifyEngineConfigChange(t *testing.T) {
	cold := ClassifyEngineConfigChange(constants.EngineEdgelet, constants.EngineDocker, "", "", nil)
	if cold != ChangeClassCold {
		t.Fatalf("expected cold, got %v", cold)
	}
	warm := ClassifyEngineConfigChange(constants.EngineDocker, constants.EngineDocker,
		"unix:///var/run/docker.sock", "unix:///run/docker/docker.sock", nil)
	if warm != ChangeClassWarm {
		t.Fatalf("expected warm, got %v", warm)
	}
	hot := ClassifyEngineConfigChange(constants.EngineDocker, constants.EngineDocker,
		"unix:///var/run/docker.sock", "unix:///var/run/docker.sock", nil)
	if hot != ChangeClassHot {
		t.Fatalf("expected hot, got %v", hot)
	}
}
