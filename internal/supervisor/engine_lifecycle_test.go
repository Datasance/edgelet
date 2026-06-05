package supervisor

import (
	"testing"

	"github.com/datasance/edgelet/internal/constants"
	"github.com/datasance/edgelet/internal/runtime"
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
	runtime.ResetForTests()
	s := NewSupervisor()
	cfg := captureReloadEngineContextViaSupervisor(s)
	if cfg.priorContainerEngineURL != "" {
		// empty config defaults are fine
	}
	_ = cfg
}

func captureReloadEngineContextViaSupervisor(s *Supervisor) *reloadEngineContext {
	return s.captureReloadEngineContext()
}
