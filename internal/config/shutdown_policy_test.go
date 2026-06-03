package config

import (
	"testing"

	"github.com/datasance/edgelet/internal/constants"
)

func TestDefaultShutdownPolicy(t *testing.T) {
	if got := DefaultShutdownPolicy(constants.EngineDocker); got != ShutdownPolicyLeaveRunning {
		t.Fatalf("docker default: got %q want %q", got, ShutdownPolicyLeaveRunning)
	}
	if got := DefaultShutdownPolicy(constants.EnginePodman); got != ShutdownPolicyLeaveRunning {
		t.Fatalf("podman default: got %q want %q", got, ShutdownPolicyLeaveRunning)
	}
	if got := DefaultShutdownPolicy(constants.EngineEdgelet); got != ShutdownPolicyDrainAll {
		t.Fatalf("edgelet default: got %q want %q", got, ShutdownPolicyDrainAll)
	}
}

func TestLeaveRunningOnControlStop(t *testing.T) {
	cfg := GetInstance()
	origEngine := cfg.ContainerEngine
	origPolicy := cfg.ShutdownPolicy
	t.Cleanup(func() {
		cfg.ContainerEngine = origEngine
		cfg.ShutdownPolicy = origPolicy
	})

	cfg.ContainerEngine = constants.EngineDocker
	cfg.ShutdownPolicy = ""
	if !cfg.LeaveRunningOnControlStop() {
		t.Fatal("expected docker implicit leave-running")
	}

	cfg.ContainerEngine = constants.EngineEdgelet
	cfg.ShutdownPolicy = ""
	if cfg.LeaveRunningOnControlStop() {
		t.Fatal("expected edgelet implicit drain-all")
	}

	cfg.ShutdownPolicy = ShutdownPolicyLeaveRunning
	if !cfg.LeaveRunningOnControlStop() {
		t.Fatal("expected explicit leave-running")
	}
}

func TestShutdownDrainTimeout(t *testing.T) {
	cfg := GetInstance()
	orig := cfg.ShutdownGracePeriodSeconds
	t.Cleanup(func() { cfg.ShutdownGracePeriodSeconds = orig })

	cfg.ShutdownGracePeriodSeconds = 45
	if got := cfg.ShutdownDrainTimeout(); got != 45 {
		t.Fatalf("got %d want 45", got)
	}
}
