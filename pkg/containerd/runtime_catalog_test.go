package iofogcontainerd

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestBuildRuntimeCatalog_DeterministicOrderAndEligibility(t *testing.T) {
	prev := lookPathForRuntimeCatalog
	t.Cleanup(func() { lookPathForRuntimeCatalog = prev })

	lookPathForRuntimeCatalog = func(file string) (string, error) {
		switch file {
		case "runc":
			return "/usr/bin/runc", nil
		case "containerd-shim-spin-v2":
			return "/usr/local/bin/containerd-shim-spin-v2", nil
		case "containerd-shim-edgelet-v2":
			return "/opt/edgelet/containerd-shim-edgelet-v2", nil
		default:
			return "", fmt.Errorf("not found: %s", file)
		}
	}

	catalog := BuildRuntimeCatalog()
	if len(catalog) != 3 {
		t.Fatalf("expected found-only catalog entries, got %d", len(catalog))
	}

	for i := 1; i < len(catalog); i++ {
		if catalog[i-1].Handler > catalog[i].Handler {
			t.Fatalf("catalog must be sorted by handler: %q > %q", catalog[i-1].Handler, catalog[i].Handler)
		}
	}

	runc, ok := runtimeEntryByHandler("runc", catalog)
	if !ok || runc.Path != "/usr/bin/runc" || runc.Family != RuntimeFamilyRunc {
		t.Fatalf("expected discovered runc entry, got %+v", runc)
	}

	wasmer, ok := runtimeEntryByHandler("wasmer", catalog)
	if ok {
		t.Fatalf("wasmer should not be present when unavailable, got %+v", wasmer)
	}

	edgelet, ok := runtimeEntryByHandler("edgelet", catalog)
	if !ok || edgelet.Family != RuntimeFamilyShim || edgelet.RuntimeType != "io.containerd.edgelet.v2" {
		t.Fatalf("expected discovered edgelet shim entry, got %+v", edgelet)
	}
}

func TestBuildRuntimeCatalog_UsesNvidiaExperimentalHyphenBinary(t *testing.T) {
	prev := lookPathForRuntimeCatalog
	t.Cleanup(func() { lookPathForRuntimeCatalog = prev })

	lookPathForRuntimeCatalog = func(file string) (string, error) {
		switch file {
		case "nvidia-container-runtime-experimental":
			return "/usr/bin/nvidia-container-runtime-experimental", nil
		}
		return "", errors.New("not found")
	}

	catalog := BuildRuntimeCatalog()
	nvidiaExperimental, ok := runtimeEntryByHandler("nvidia-experimental", catalog)
	if !ok {
		t.Fatalf("expected nvidia-experimental entry")
	}
	if nvidiaExperimental.Binary != "nvidia-container-runtime-experimental" {
		t.Fatalf("expected hyphenated binary candidate, got %q", nvidiaExperimental.Binary)
	}
}

func TestValidateRuntimeHandlerEligibility_ReturnsClearErrors(t *testing.T) {
	catalog := []RuntimeCatalogEntry{
		{
			Handler:     "spin",
			Binary:      "containerd-shim-spin-v2",
			Path:        "/usr/local/bin/containerd-shim-spin-v2",
			RuntimeType: "io.containerd.spin.v2",
			Family:      RuntimeFamilyShim,
		},
	}

	if _, err := ValidateRuntimeHandlerEligibility("spin", catalog); err != nil {
		t.Fatalf("expected spin to be eligible, got %v", err)
	}

	_, err := ValidateRuntimeHandlerEligibility("wasmer", catalog)
	if err == nil {
		t.Fatal("expected error for unavailable wasmer handler")
	}
	var unavailable *ErrRuntimeHandlerUnavailable
	if !errors.As(err, &unavailable) {
		t.Fatalf("expected ErrRuntimeHandlerUnavailable, got %T", err)
	}
	if !strings.Contains(err.Error(), "handler \"wasmer\" is not available in PATH") {
		t.Fatalf("expected clear unavailable message, got %q", err.Error())
	}

	_, err = ValidateRuntimeHandlerEligibility("unknown", catalog)
	if err == nil {
		t.Fatal("expected error for unsupported handler")
	}
	if !strings.Contains(err.Error(), "handler \"unknown\" is not supported") {
		t.Fatalf("expected clear unsupported message, got %q", err.Error())
	}
}
