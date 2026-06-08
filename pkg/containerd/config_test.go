//go:build linux && cgo

package edgeletcontainerdd

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/datasance/edgelet/internal/constants"
)

func TestGenerateConfigUsesCanonicalRuntimeEntries(t *testing.T) {
	prevLookPath := lookPathForRuntimeCatalog
	t.Cleanup(func() {
		lookPathForRuntimeCatalog = prevLookPath
	})
	lookPathForRuntimeCatalog = func(file string) (string, error) {
		return "", errors.New("not found")
	}

	cfg := generateConfig()
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

	crun, ok := runtimes["crun"].(map[string]any)
	if !ok {
		t.Fatal("type assertion failed for crun")
	}
	if got := crun["cni_conf_dir"]; got != constants.EdgeletCNIConfDir {
		t.Fatalf("crun cni_conf_dir mismatch: got=%v want=%s", got, constants.EdgeletCNIConfDir)
	}
	crunOpts, ok := crun["options"].(map[string]any)
	if !ok {
		t.Fatal("type assertion failed for crunOpts")
	}
	if got := crunOpts["BinaryName"]; got != filepath.Join(constants.EdgeletContainerdBinDir, "crun") {
		t.Fatalf("crun BinaryName mismatch: got=%v want=%s", got, filepath.Join(constants.EdgeletContainerdBinDir, "crun"))
	}
	if _, ok := runtimes["crun-local"]; ok {
		t.Fatal("crun-local runtime should not be registered")
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

func TestGenerateConfigUsesScopeSelectorCNIConf(t *testing.T) {
	cfg := generateConfig()
	plugins, ok := cfg["plugins"].(map[string]any)
	if !ok {
		t.Fatal("type assertion failed for plugins")
	}
	criRuntime, ok := plugins["io.containerd.cri.v1.runtime"].(map[string]any)
	if !ok {
		t.Fatal("type assertion failed for criRuntime")
	}
	cni, ok := criRuntime["cni"].(map[string]any)
	if !ok {
		t.Fatal("type assertion failed for cni")
	}
	if got := cni["conf_dir"]; got != constants.EdgeletCNIConfDir {
		t.Fatalf("cni conf_dir mismatch: got=%v want=%s", got, constants.EdgeletCNIConfDir)
	}
	if got := cni["max_conf_num"]; got != 1 {
		t.Fatalf("cni max_conf_num mismatch: got=%v want=1", got)
	}
}

func TestGenerateConfigProjectsDiscoveredShimFamilyRuntimes(t *testing.T) {
	prevLookPath := lookPathForRuntimeCatalog
	t.Cleanup(func() {
		lookPathForRuntimeCatalog = prevLookPath
	})
	lookPathForRuntimeCatalog = func(file string) (string, error) {
		if file == "containerd-shim-spin-v2" {
			return "/opt/spin/containerd-shim-spin-v2", nil
		}
		if file == "containerd-shim-edgelet-v2" {
			return "/opt/edgelet/containerd-shim-edgelet-v2", nil
		}
		return "", errors.New("not found")
	}

	cfg := generateConfig()
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

	spin, ok := runtimes["spin"].(map[string]any)
	if !ok {
		t.Fatal("expected spin runtime entry")
	}
	if got := spin["runtime_type"]; got != "io.containerd.spin.v2" {
		t.Fatalf("spin runtime_type mismatch: got=%v want=io.containerd.spin.v2", got)
	}
	if got := spin["runtime_path"]; got != "/opt/spin/containerd-shim-spin-v2" {
		t.Fatalf("spin runtime_path mismatch: got=%v want=/opt/spin/containerd-shim-spin-v2", got)
	}
	if _, ok := spin["options"]; ok {
		t.Fatal("spin shim runtime should not include runc options")
	}

	edgeletWasmtime, ok := runtimes["edgelet-wasmtime"].(map[string]any)
	if !ok {
		t.Fatal("expected edgelet-wasmtime runtime entry")
	}
	if got := edgeletWasmtime["runtime_type"]; got != "io.containerd.edgelet.v2" {
		t.Fatalf("edgelet-wasmtime runtime_type mismatch: got=%v want=io.containerd.edgelet.v2", got)
	}
	if got := edgeletWasmtime["runtime_path"]; got != "/opt/edgelet/containerd-shim-edgelet-v2" {
		t.Fatalf("edgelet-wasmtime runtime_path mismatch: got=%v", got)
	}
	if _, ok := edgeletWasmtime["options"]; ok {
		t.Fatal("edgelet-wasmtime shim runtime should not include runc options")
	}
	if got := edgeletWasmtime["cni_conf_dir"]; got != constants.EdgeletCNIConfDir {
		t.Fatalf("edgelet-wasmtime cni_conf_dir mismatch: got=%v want=%s", got, constants.EdgeletCNIConfDir)
	}
	if _, ok := runtimes["edgelet-wasmtime-local"]; ok {
		t.Fatal("edgelet-wasmtime-local runtime entry must not be generated")
	}
}

func TestGenerateConfigProjectsDiscoveredRuncFamilyRuntimeWithBinaryName(t *testing.T) {
	prevLookPath := lookPathForRuntimeCatalog
	t.Cleanup(func() {
		lookPathForRuntimeCatalog = prevLookPath
	})
	lookPathForRuntimeCatalog = func(file string) (string, error) {
		if file == "nvidia-container-runtime" {
			return "/usr/bin/nvidia-container-runtime", nil
		}
		return "", errors.New("not found")
	}

	cfg := generateConfig()
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

	nvidia, ok := runtimes["nvidia"].(map[string]any)
	if !ok {
		t.Fatal("expected nvidia runtime entry")
	}
	if got := nvidia["runtime_type"]; got != "io.containerd.runc.v2" {
		t.Fatalf("nvidia runtime_type mismatch: got=%v want=io.containerd.runc.v2", got)
	}
	if got := nvidia["runtime_path"]; got != filepath.Join(constants.EdgeletContainerdBinDir, "containerd-shim-runc-v2") {
		t.Fatalf("nvidia runtime_path mismatch: got=%v", got)
	}
	opts, ok := nvidia["options"].(map[string]any)
	if !ok {
		t.Fatal("expected nvidia runtime options for runc family")
	}
	if got := opts["BinaryName"]; got != "/usr/bin/nvidia-container-runtime" {
		t.Fatalf("nvidia BinaryName mismatch: got=%v want=/usr/bin/nvidia-container-runtime", got)
	}
}

func TestRenderContainerdConfig_IsDeterministicAcrossRepeatedRuns(t *testing.T) {
	prevLookPath := lookPathForRuntimeCatalog
	prevExtensionPath := containerdTemplateExtensionPath
	t.Cleanup(func() {
		lookPathForRuntimeCatalog = prevLookPath
		containerdTemplateExtensionPath = prevExtensionPath
	})

	containerdTemplateExtensionPath = filepath.Join(t.TempDir(), "missing-config.toml.tmpl")
	lookPathForRuntimeCatalog = func(file string) (string, error) {
		if file == "containerd-shim-edgelet-v2" {
			return "/opt/edgelet/containerd-shim-edgelet-v2", nil
		}
		return "", errors.New("not found")
	}

	first, err := renderContainerdConfig()
	if err != nil {
		t.Fatalf("first render failed: %v", err)
	}
	second, err := renderContainerdConfig()
	if err != nil {
		t.Fatalf("second render failed: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("expected deterministic render output across repeated runs")
	}
}

func TestRenderContainerdTemplatePipeline_AppendsOptionalExtension(t *testing.T) {
	prevExtensionPath := containerdTemplateExtensionPath
	t.Cleanup(func() {
		containerdTemplateExtensionPath = prevExtensionPath
	})

	extDir := t.TempDir()
	containerdTemplateExtensionPath = filepath.Join(extDir, "config.toml.tmpl")
	if err := os.WriteFile(containerdTemplateExtensionPath, []byte(`[plugins."io.containerd.debug.v1"]
  level = "info"`), 0644); err != nil {
		t.Fatalf("failed to write extension template: %v", err)
	}

	rendered, err := renderContainerdTemplatePipeline([]byte("version = 3\n"))
	if err != nil {
		t.Fatalf("render pipeline failed: %v", err)
	}
	got := string(rendered)
	if !bytes.Contains(rendered, []byte("version = 3")) {
		t.Fatalf("expected base config content in rendered output: %q", got)
	}
	if !bytes.Contains(rendered, []byte(`plugins."io.containerd.debug.v1"`)) {
		t.Fatalf("expected extension content in rendered output: %q", got)
	}
}

func TestWriteFileAtomicallyAndLKGHelpers(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.toml")
	content := []byte("version = 3\n")
	if err := writeFileAtomically(configPath, content, 0644); err != nil {
		t.Fatalf("atomic write failed: %v", err)
	}
	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config failed: %v", err)
	}
	if !bytes.Equal(raw, content) {
		t.Fatalf("unexpected config content: got=%q want=%q", string(raw), string(content))
	}

	if err := writeLastKnownGoodConfig(configPath, content); err != nil {
		t.Fatalf("write lkg failed: %v", err)
	}
	lkg, err := readLastKnownGoodConfig(configPath)
	if err != nil {
		t.Fatalf("read lkg failed: %v", err)
	}
	if !bytes.Equal(lkg, content) {
		t.Fatalf("unexpected lkg content: got=%q want=%q", string(lkg), string(content))
	}
}
