//go:build linux

package iofogcontainerd

import (
	"path/filepath"
	"testing"

	"github.com/eclipse-iofog/agent/internal/constants"
)

func TestGenerateConfigUsesSplitCNIDirsForRuntimes(t *testing.T) {
	cfg := generateConfig()
	plugins := cfg["plugins"].(map[string]any)
	criRuntime := plugins["io.containerd.cri.v1.runtime"].(map[string]any)
	containerdCfg := criRuntime["containerd"].(map[string]any)
	runtimes := containerdCfg["runtimes"].(map[string]any)

	crun := runtimes["crun"].(map[string]any)
	if got := crun["cni_conf_dir"]; got != constants.IofogManagedCNIConfDir {
		t.Fatalf("crun cni_conf_dir mismatch: got=%v want=%s", got, constants.IofogManagedCNIConfDir)
	}
	crunOpts := crun["options"].(map[string]any)
	if got := crunOpts["BinaryName"]; got != filepath.Join(constants.IofogContainerdBinDir, "crun") {
		t.Fatalf("crun BinaryName mismatch: got=%v want=%s", got, filepath.Join(constants.IofogContainerdBinDir, "crun"))
	}
	crunLocal := runtimes["crun-local"].(map[string]any)
	if got := crunLocal["cni_conf_dir"]; got != constants.IofogLocalCNIConfDir {
		t.Fatalf("crun-local cni_conf_dir mismatch: got=%v want=%s", got, constants.IofogLocalCNIConfDir)
	}
	crunLocalOpts := crunLocal["options"].(map[string]any)
	if got := crunLocalOpts["BinaryName"]; got != filepath.Join(constants.IofogContainerdBinDir, "crun") {
		t.Fatalf("crun-local BinaryName mismatch: got=%v want=%s", got, filepath.Join(constants.IofogContainerdBinDir, "crun"))
	}

	if got := containerdCfg["default_runtime_name"]; got != "crun" {
		t.Fatalf("default_runtime_name mismatch: got=%v want=crun", got)
	}
	if _, ok := runtimes["spin"]; ok {
		t.Fatal("spin runtime should not be registered")
	}
	if _, ok := runtimes["spin-local"]; ok {
		t.Fatal("spin-local runtime should not be registered")
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
