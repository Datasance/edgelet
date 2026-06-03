package config

import (
	"strings"

	"github.com/datasance/edgelet/internal/constants"
)

const (
	// ShutdownPolicyLeaveRunning skips runtime drain on control-plane stop; MS reattach on start.
	ShutdownPolicyLeaveRunning = "leave-running"
	// ShutdownPolicyDrainAll stops managed MS containers during shutdown (embedded monolith / data-plane stop).
	ShutdownPolicyDrainAll = "drain-all"
)

// DefaultShutdownPolicy returns the engine-appropriate default when shutdownPolicy is unset.
func DefaultShutdownPolicy(containerEngine string) string {
	switch strings.ToLower(strings.TrimSpace(containerEngine)) {
	case constants.EngineDocker, constants.EnginePodman:
		return ShutdownPolicyLeaveRunning
	default:
		return ShutdownPolicyDrainAll
	}
}

// LeaveRunningOnControlStop reports whether control-plane stop should skip runtime drain.
func (c *Config) LeaveRunningOnControlStop() bool {
	if c == nil {
		return false
	}
	policy := strings.ToLower(strings.TrimSpace(c.ShutdownPolicy))
	if policy == "" {
		policy = DefaultShutdownPolicy(c.ContainerEngine)
	}
	return policy == ShutdownPolicyLeaveRunning
}

// ShutdownDrainTimeout returns the configured runtime drain deadline for maintenance/data-plane stops.
func (c *Config) ShutdownDrainTimeout() int {
	if c == nil || c.ShutdownGracePeriodSeconds < 1 {
		return 90
	}
	return c.ShutdownGracePeriodSeconds
}
