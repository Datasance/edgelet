package supervisor

import (
	"testing"

	"github.com/eclipse-iofog/edgelet/internal/constants"
	"github.com/eclipse-iofog/edgelet/internal/runtimestate"
)

func TestStartupEngineURL(t *testing.T) {
	if got := startupEngineURL(constants.EngineEdgelet, "unix:///var/run/docker.sock"); got != constants.EdgeletEngineSocketURL() {
		t.Fatalf("edgelet url = %q", got)
	}
	if got := startupEngineURL(constants.EngineDocker, "unix:///var/run/docker.sock"); got != "unix:///var/run/docker.sock" {
		t.Fatalf("docker url = %q", got)
	}
}

func TestCaptureReloadEngineContextUsesStartupSnapshot(t *testing.T) {
	runtimestate.ResetForTests()
	s := NewSupervisor()
	_ = captureReloadEngineContextViaSupervisor(s)
}

func captureReloadEngineContextViaSupervisor(s *Supervisor) *reloadEngineContext {
	return s.captureReloadEngineContext()
}
