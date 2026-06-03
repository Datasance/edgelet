package cgroups

import (
	"sort"
	"strings"
	"sync"
)

// Mode describes the host cgroup hierarchy layout.
type Mode string

const (
	ModeV1      Mode = "v1"
	ModeV2      Mode = "v2"
	ModeHybrid  Mode = "hybrid"
	ModeUnknown Mode = "unknown"
)

// Driver is the cgroup driver selected for embedded containerd/crun.
type Driver string

const (
	DriverSystemd  Driver = "systemd"
	DriverCgroupfs Driver = "cgroupfs"
)

// CgroupPolicy is the resolved cgroup bootstrap policy for embedded edgelet.
type CgroupPolicy struct {
	Mode                 Mode
	Driver               Driver
	Nested               bool
	SystemdCgroup        bool
	UnifiedMountpoint    string
	AgentCgroupPath      string
	ContainerdCgroupPath string
	DelegatedControllers []string
	HybridWarning        string
}

// Snapshot exposes cgroup diagnostics for status reporting.
type Snapshot struct {
	Mode                 string
	Driver               string
	Nested               bool
	DelegatedControllers string
	AgentCgroupPath      string
	ContainerdCgroupPath string
}

var (
	globalMu     sync.RWMutex
	globalPolicy *CgroupPolicy
)

// SetGlobalPolicy records the active policy after bootstrap.
func SetGlobalPolicy(p *CgroupPolicy) {
	globalMu.Lock()
	defer globalMu.Unlock()
	globalPolicy = p
}

// GetGlobalPolicy returns the active policy, if bootstrap completed.
func GetGlobalPolicy() *CgroupPolicy {
	globalMu.RLock()
	defer globalMu.RUnlock()
	return globalPolicy
}

// SnapshotFromPolicy builds a status snapshot from policy.
func SnapshotFromPolicy(p *CgroupPolicy) Snapshot {
	if p == nil {
		return Snapshot{
			Mode:   string(ModeUnknown),
			Driver: "",
		}
	}
	return Snapshot{
		Mode:                 string(p.Mode),
		Driver:               string(p.Driver),
		Nested:               p.Nested,
		DelegatedControllers: joinControllers(p.DelegatedControllers),
		AgentCgroupPath:      p.AgentCgroupPath,
		ContainerdCgroupPath: p.ContainerdCgroupPath,
	}
}

// GetSnapshot returns diagnostics for the active policy.
func GetSnapshot() Snapshot {
	return SnapshotFromPolicy(GetGlobalPolicy())
}

func joinControllers(controllers []string) string {
	if len(controllers) == 0 {
		return ""
	}
	out := append([]string(nil), controllers...)
	sort.Strings(out)
	return strings.Join(out, ",")
}
