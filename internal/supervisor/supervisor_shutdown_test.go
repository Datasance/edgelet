package supervisor

import (
	"testing"

	"github.com/eclipse-iofog/edgelet/internal/config"
	"github.com/eclipse-iofog/edgelet/internal/constants"
)

func TestShouldDrainRuntimeOnControlStop(t *testing.T) {
	cfg := config.GetInstance()
	origEngine := cfg.ContainerEngine
	origPolicy := cfg.ShutdownPolicy
	t.Cleanup(func() {
		cfg.ContainerEngine = origEngine
		cfg.ShutdownPolicy = origPolicy
	})

	s := NewSupervisor()

	cfg.ContainerEngine = constants.EngineDocker
	cfg.ShutdownPolicy = config.ShutdownPolicyLeaveRunning
	s.SetContainerdAttachOnly(false)
	if s.shouldDrainRuntimeOnControlStop() {
		t.Fatal("expected no drain for docker leave-running")
	}

	cfg.ContainerEngine = constants.EngineEdgelet
	cfg.ShutdownPolicy = config.ShutdownPolicyDrainAll
	s.SetContainerdAttachOnly(false)
	if !s.shouldDrainRuntimeOnControlStop() {
		t.Fatal("expected drain for embedded monolith drain-all")
	}

	s.SetContainerdAttachOnly(true)
	if s.shouldDrainRuntimeOnControlStop() {
		t.Fatal("expected no drain when attach-only")
	}

	cfg.ShutdownPolicy = config.ShutdownPolicyLeaveRunning
	s.SetContainerdAttachOnly(false)
	if s.shouldDrainRuntimeOnControlStop() {
		t.Fatal("expected no drain for explicit leave-running")
	}
}
