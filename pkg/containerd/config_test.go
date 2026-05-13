//go:build linux

package iofogcontainerd

import (
	"testing"

	"github.com/eclipse-iofog/agent/internal/constants"
)

func TestGenerateConfigUsesSplitCNIDirsForRuntimes(t *testing.T) {
	cfg := generateConfig()
	plugins := cfg["plugins"].(map[string]any)
	criRuntime := plugins["io.containerd.cri.v1.runtime"].(map[string]any)
	containerdCfg := criRuntime["containerd"].(map[string]any)
	runtimes := containerdCfg["runtimes"].(map[string]any)

	runc := runtimes["runc"].(map[string]any)
	if got := runc["cni_conf_dir"]; got != constants.IofogManagedCNIConfDir {
		t.Fatalf("runc cni_conf_dir mismatch: got=%v want=%s", got, constants.IofogManagedCNIConfDir)
	}
	runcLocal := runtimes["runc-local"].(map[string]any)
	if got := runcLocal["cni_conf_dir"]; got != constants.IofogLocalCNIConfDir {
		t.Fatalf("runc-local cni_conf_dir mismatch: got=%v want=%s", got, constants.IofogLocalCNIConfDir)
	}
}

func TestGenerateConfigAllowsMultipleCNIConfs(t *testing.T) {
	cfg := generateConfig()
	plugins := cfg["plugins"].(map[string]any)
	criRuntime := plugins["io.containerd.cri.v1.runtime"].(map[string]any)
	cni := criRuntime["cni"].(map[string]any)
	if got := cni["max_conf_num"]; got != 2 {
		t.Fatalf("cni max_conf_num mismatch: got=%v want=2", got)
	}
}
