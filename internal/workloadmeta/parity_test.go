package workloadmeta

import "testing"

// Ensures docker, podman, and iofog writers share the same canonical label *keys*
// (values differ only where intended: runtime-engine string and sandbox-id presence).
func TestBuildLabels_keysParityAcrossRuntimeEngines(t *testing.T) {
	wantKeys := []string{
		LabelAppName,
		LabelAppInstance,
		LabelAppPartOf,
		LabelAppManagedBy,
		LabelMicroserviceUID,
		LabelNodeUID,
		LabelScope,
		LabelRuntimeEngine,
		LabelRole,
		LabelSystem,
		LabelHostNetwork,
	}

	for _, rt := range []string{RuntimeEngineDocker, RuntimeEnginePodman, RuntimeEngineIofog} {
		in := BuildInput{
			MicroserviceUUID: "ms-parity",
			MicroserviceName: "parity-svc",
			ApplicationName:  "parity-app",
			NodeUUID:         "node-parity",
			RuntimeEngine:    rt,
			IsRouter:         true,
			IsNats:           true,
			HostNetwork:      true,
			IsSystem:         false,
			SandboxID:        "sandbox-parity",
		}
		labels := BuildLabels(in)

		for _, k := range wantKeys {
			if _, ok := labels[k]; !ok {
				t.Fatalf("runtime=%q missing required label key %q; labels=%v", rt, k, labels)
			}
		}
		if labels[LabelRuntimeEngine] != normalizeRuntimeEngine(rt) {
			t.Fatalf("runtime=%q: expected normalized runtime label %q, got %q", rt, normalizeRuntimeEngine(rt), labels[LabelRuntimeEngine])
		}
		if labels[LabelSandboxID] != "sandbox-parity" {
			t.Fatalf("runtime=%q: expected sandbox id label, got %q", rt, labels[LabelSandboxID])
		}
		for _, lk := range RemovedLegacyLabelKeys {
			if v := labels[lk]; v != "" {
				t.Fatalf("runtime=%q: removed legacy label %q must be absent, got %q", rt, lk, v)
			}
		}
	}
}
