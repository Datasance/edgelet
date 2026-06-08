//revive:disable:package-directory-mismatch
package edgeletcontainerdd

import (
	"cmp"
	"errors"
	"fmt"
	"os/exec"
	"slices"
	"strings"
)

var lookPathForRuntimeCatalog = exec.LookPath

var errRuntimeHandlerUnavailable = errors.New("runtime handler unavailable")

type RuntimeFamily string

const (
	RuntimeFamilyRunc RuntimeFamily = "runc"
	RuntimeFamilyShim RuntimeFamily = "shim"
)

type runtimeSpec struct {
	Handler     string
	RuntimeType string
	Family      RuntimeFamily
	Candidates  []string
}

var runtimeSpecs = map[string]runtimeSpec{
	"runc": {
		Handler:     "runc",
		RuntimeType: "io.containerd.runc.v2",
		Family:      RuntimeFamilyRunc,
		Candidates:  []string{"runc"},
	},
	"nvidia": {
		Handler:     "nvidia",
		RuntimeType: "io.containerd.runc.v2",
		Family:      RuntimeFamilyRunc,
		Candidates:  []string{"nvidia-container-runtime"},
	},
	"nvidia-experimental": {
		Handler:     "nvidia-experimental",
		RuntimeType: "io.containerd.runc.v2",
		Family:      RuntimeFamilyRunc,
		Candidates:  []string{"nvidia-container-runtime-experimental", "nvidia-container-runtime"},
	},
	"nvidia-cdi": {
		Handler:     "nvidia-cdi",
		RuntimeType: "io.containerd.runc.v2",
		Family:      RuntimeFamilyRunc,
		Candidates:  []string{"nvidia-container-runtime.cdi", "nvidia-container-runtime"},
	},
	"lunatic": {
		Handler:     "lunatic",
		RuntimeType: "io.containerd.lunatic.v1",
		Family:      RuntimeFamilyShim,
		Candidates:  []string{"containerd-shim-lunatic-v1", "containerd-shim-lunatic-v2", "lunatic"},
	},
	"slight": {
		Handler:     "slight",
		RuntimeType: "io.containerd.slight.v1",
		Family:      RuntimeFamilyShim,
		Candidates:  []string{"containerd-shim-slight-v1", "containerd-shim-slight-v2", "slight"},
	},
	"spin": {
		Handler:     "spin",
		RuntimeType: "io.containerd.spin.v2",
		Family:      RuntimeFamilyShim,
		Candidates:  []string{"containerd-shim-spin-v2", "containerd-shim-spin-v1", "spin"},
	},
	"wws": {
		Handler:     "wws",
		RuntimeType: "io.containerd.wws.v1",
		Family:      RuntimeFamilyShim,
		Candidates:  []string{"containerd-shim-wws-v1", "containerd-shim-wws-v2", "wws"},
	},
	"wasmedge": {
		Handler:     "wasmedge",
		RuntimeType: "io.containerd.wasmedge.v1",
		Family:      RuntimeFamilyShim,
		Candidates:  []string{"containerd-shim-wasmedge-v1", "containerd-shim-wasmedge-v2", "wasmedge"},
	},
	"wasmer": {
		Handler:     "wasmer",
		RuntimeType: "io.containerd.wasmer.v1",
		Family:      RuntimeFamilyShim,
		Candidates:  []string{"containerd-shim-wasmer-v1", "containerd-shim-wasmer-v2", "wasmer"},
	},
	"wasmtime": {
		Handler:     "wasmtime",
		RuntimeType: "io.containerd.wasmtime.v1",
		Family:      RuntimeFamilyShim,
		Candidates:  []string{"containerd-shim-wasmtime-v1", "containerd-shim-wasmtime-v2", "wasmtime"},
	},
	"edgelet-wasmtime": {
		Handler:     "edgelet-wasmtime",
		RuntimeType: "io.containerd.edgelet.v2",
		Family:      RuntimeFamilyShim,
		Candidates:  []string{"containerd-shim-edgelet-v2", "containerd-shim-edgelet-wasm-v2", "containerd-shim-edgelet", "edgelet-wasm"},
	},
}

// RuntimeCatalogEntry is one runtime handler detection result.
type RuntimeCatalogEntry struct {
	Handler     string
	Binary      string
	Path        string
	RuntimeType string
	Family      RuntimeFamily
}

// ErrRuntimeHandlerUnavailable indicates a requested handler is missing or not eligible.
type ErrRuntimeHandlerUnavailable struct {
	Handler    string
	Candidates []string
}

func (e *ErrRuntimeHandlerUnavailable) Error() string {
	handler := strings.TrimSpace(strings.ToLower(e.Handler))
	candidates := append([]string{}, e.Candidates...)
	if len(candidates) == 0 {
		return fmt.Sprintf("%v: handler %q is not supported", errRuntimeHandlerUnavailable, handler)
	}
	return fmt.Sprintf("%v: handler %q is not available in PATH (tried: %s)", errRuntimeHandlerUnavailable, handler, strings.Join(candidates, ", "))
}

func (e *ErrRuntimeHandlerUnavailable) Unwrap() error {
	return errRuntimeHandlerUnavailable
}

// BuildRuntimeCatalog discovers supported runtimes from PATH.
// Only discovered handlers are returned.
func BuildRuntimeCatalog() []RuntimeCatalogEntry {
	entries := make([]RuntimeCatalogEntry, 0, len(runtimeSpecs))
	entries = findRunCContainerRuntime(entries)
	entries = findNvidiaContainerRuntimes(entries)
	entries = findWasiRuntimes(entries)
	slices.SortFunc(entries, func(a, b RuntimeCatalogEntry) int {
		return cmp.Compare(a.Handler, b.Handler)
	})
	return entries
}

// ValidateRuntimeHandlerEligibility returns a runtime entry only when handler
// exists in catalog and is eligible.
func ValidateRuntimeHandlerEligibility(handler string, catalog []RuntimeCatalogEntry) (RuntimeCatalogEntry, error) {
	normalized := strings.TrimSpace(strings.ToLower(handler))
	if _, ok := runtimeSpecs[normalized]; !ok {
		return RuntimeCatalogEntry{}, &ErrRuntimeHandlerUnavailable{
			Handler:    normalized,
			Candidates: runtimeHandlerLookupCandidates(normalized),
		}
	}
	entry, ok := runtimeEntryByHandler(normalized, catalog)
	if !ok {
		return RuntimeCatalogEntry{}, &ErrRuntimeHandlerUnavailable{
			Handler:    normalized,
			Candidates: runtimeHandlerLookupCandidates(normalized),
		}
	}
	return entry, nil
}

func runtimeEntryByHandler(handler string, catalog []RuntimeCatalogEntry) (RuntimeCatalogEntry, bool) {
	for _, item := range catalog {
		if item.Handler == handler {
			return item, true
		}
	}
	return RuntimeCatalogEntry{}, false
}

func runtimeHandlerLookupCandidates(handler string) []string {
	spec, ok := runtimeSpecs[handler]
	if !ok {
		return nil
	}
	return append([]string{}, spec.Candidates...)
}

func findRunCContainerRuntime(entries []RuntimeCatalogEntry) []RuntimeCatalogEntry {
	potential := map[string]runtimeSpec{
		"runc": runtimeSpecs["runc"],
	}
	return searchForRuntimes(potential, entries)
}

func findNvidiaContainerRuntimes(entries []RuntimeCatalogEntry) []RuntimeCatalogEntry {
	potential := map[string]runtimeSpec{
		"nvidia":              runtimeSpecs["nvidia"],
		"nvidia-cdi":          runtimeSpecs["nvidia-cdi"],
		"nvidia-experimental": runtimeSpecs["nvidia-experimental"],
	}
	return searchForRuntimes(potential, entries)
}

func findWasiRuntimes(entries []RuntimeCatalogEntry) []RuntimeCatalogEntry {
	potential := map[string]runtimeSpec{
		"edgelet-wasmtime": runtimeSpecs["edgelet-wasmtime"],
		"lunatic":          runtimeSpecs["lunatic"],
		"slight":           runtimeSpecs["slight"],
		"spin":             runtimeSpecs["spin"],
		"wasmedge":         runtimeSpecs["wasmedge"],
		"wasmer":           runtimeSpecs["wasmer"],
		"wasmtime":         runtimeSpecs["wasmtime"],
		"wws":              runtimeSpecs["wws"],
	}
	return searchForRuntimes(potential, entries)
}

func searchForRuntimes(potential map[string]runtimeSpec, entries []RuntimeCatalogEntry) []RuntimeCatalogEntry {
	handlers := make([]string, 0, len(potential))
	for runtimeName := range potential {
		handlers = append(handlers, runtimeName)
	}
	slices.Sort(handlers)
	for _, runtimeName := range handlers {
		spec := potential[runtimeName]
		for _, candidate := range spec.Candidates {
			path, err := lookPathForRuntimeCatalog(candidate)
			if err != nil {
				continue
			}
			entries = append(entries, RuntimeCatalogEntry{
				Handler:     spec.Handler,
				Binary:      candidate,
				Path:        path,
				RuntimeType: spec.RuntimeType,
				Family:      spec.Family,
			})
			break
		}
	}
	return entries
}
