//go:build linux && cgo

package containerd

import (
	"testing"

	"github.com/eclipse-iofog/edgelet/internal/cgroups"
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

	plugins, ok := cfg["plugins"].(map[string]any)
	if !ok {
		t.Fatal("type assertion failed for plugins")
	}
	criRuntime, ok := plugins["io.containerd.cri.v1.runtime"].(map[string]any)
	if !ok {
		t.Fatal("type assertion failed for criRuntime")
	}
	containerdCfg, ok := criRuntime["containerd"].(map[string]any)
	if !ok {
		t.Fatal("type assertion failed for containerdCfg")
	}
	runtimes, ok := containerdCfg["runtimes"].(map[string]any)
	if !ok {
		t.Fatal("type assertion failed for runtimes")
	}
	crunOpts, ok := runtimes["crun"].(map[string]any)["options"].(map[string]any)
	if !ok {
		t.Fatal("type assertion failed for crunOpts")
	}
	if v, ok := crunOpts["SystemdCgroup"].(bool); ok && v {
		t.Fatalf("crun SystemdCgroup = %v want false", v)
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

	plugins, ok := cfg["plugins"].(map[string]any)
	if !ok {
		t.Fatal("type assertion failed for plugins")
	}
	criRuntime, ok := plugins["io.containerd.cri.v1.runtime"].(map[string]any)
	if !ok {
		t.Fatal("type assertion failed for criRuntime")
	}
	containerdCfg, ok := criRuntime["containerd"].(map[string]any)
	if !ok {
		t.Fatal("type assertion failed for containerdCfg")
	}
	runtimes, ok := containerdCfg["runtimes"].(map[string]any)
	if !ok {
		t.Fatal("type assertion failed for runtimes")
	}
	crunOpts, ok := runtimes["crun"].(map[string]any)["options"].(map[string]any)
	if !ok {
		t.Fatal("type assertion failed for crunOpts")
	}
	if v, ok := crunOpts["SystemdCgroup"].(bool); ok && v {
		t.Fatalf("crun SystemdCgroup = %v want false", v)
	}
}
