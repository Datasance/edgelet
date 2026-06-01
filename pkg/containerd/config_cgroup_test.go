//go:build linux && cgo

package edgeletcontainerdd

import (
	"testing"

	"github.com/datasance/edgelet/internal/cgroups"
)

func TestGenerateConfigSystemdDriverOmitsCgroupPath(t *testing.T) {
	prev := cgroups.GetGlobalPolicy()
	t.Cleanup(func() { cgroups.SetGlobalPolicy(prev) })

	cgroups.SetGlobalPolicy(&cgroups.CgroupPolicy{
		Mode:                 cgroups.ModeV2,
		Driver:               cgroups.DriverSystemd,
		SystemdCgroup:        false,
		ContainerdCgroupPath: "/edgelet/agent/containerd",
	})

	cfg := generateConfig()
	if got := cfg["cgroup"].(map[string]any)["path"]; got != "" {
		t.Fatalf("cgroup.path = %q want empty for systemd driver", got)
	}

	plugins := cfg["plugins"].(map[string]any)
	criRuntime := plugins["io.containerd.cri.v1.runtime"].(map[string]any)
	containerdCfg := criRuntime["containerd"].(map[string]any)
	runtimes := containerdCfg["runtimes"].(map[string]any)
	crunOpts := runtimes["crun"].(map[string]any)["options"].(map[string]any)
	if got := crunOpts["SystemdCgroup"]; got != false {
		t.Fatalf("crun SystemdCgroup = %v want false", got)
	}
}

func TestGenerateConfigCgroupfsDriverSetsCgroupPath(t *testing.T) {
	prev := cgroups.GetGlobalPolicy()
	t.Cleanup(func() { cgroups.SetGlobalPolicy(prev) })

	cgroups.SetGlobalPolicy(&cgroups.CgroupPolicy{
		Mode:                 cgroups.ModeV2,
		Driver:               cgroups.DriverCgroupfs,
		SystemdCgroup:        false,
		ContainerdCgroupPath: "/edgelet/agent/containerd",
	})

	cfg := generateConfig()
	if got := cfg["cgroup"].(map[string]any)["path"]; got != "/edgelet/agent/containerd" {
		t.Fatalf("cgroup.path = %v", got)
	}

	plugins := cfg["plugins"].(map[string]any)
	criRuntime := plugins["io.containerd.cri.v1.runtime"].(map[string]any)
	containerdCfg := criRuntime["containerd"].(map[string]any)
	runtimes := containerdCfg["runtimes"].(map[string]any)
	crunOpts := runtimes["crun"].(map[string]any)["options"].(map[string]any)
	if got := crunOpts["SystemdCgroup"]; got != false {
		t.Fatalf("crun SystemdCgroup = %v want false", got)
	}
}
