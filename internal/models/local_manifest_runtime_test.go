package models

import (
	"testing"

	"github.com/eclipse-iofog/edgelet/internal/workloadmeta"
)

func TestBuildMicroserviceFromLocalManifestUsesLocalApplicationScope(t *testing.T) {
	doc := validLocalDeployManifestForTest("local-svc")
	ms := BuildMicroserviceFromLocalManifest(doc, "dep-1", "nginx:latest")

	if ms.ApplicationName != workloadmeta.LocalDeployApplicationName {
		t.Fatalf("expected application %q, got %q", workloadmeta.LocalDeployApplicationName, ms.ApplicationName)
	}

	if got := workloadmeta.ResolveScope(ms.ApplicationName, ms.HostNetworkMode); got != workloadmeta.ScopeLocal {
		t.Fatalf("expected local scope for local deploy, got %q", got)
	}
}

func TestBuildMicroserviceFromLocalManifestHostNetworkBypassesLocalScope(t *testing.T) {
	doc := validLocalDeployManifestForTest("local-hostnet")
	doc.Spec.Container.HostNetworkMode = true
	ms := BuildMicroserviceFromLocalManifest(doc, "dep-2", "nginx:latest")

	if got := workloadmeta.ResolveScope(ms.ApplicationName, ms.HostNetworkMode); got != workloadmeta.ScopeManaged {
		t.Fatalf("expected managed scope for host-network local deploy, got %q", got)
	}
}

func TestBuildMicroserviceFromLocalManifestConvertsMemoryLimitMiBToBytes(t *testing.T) {
	doc := validLocalDeployManifestForTest("local-mem")
	doc.Spec.Container.MemoryLimit = 512
	ms := BuildMicroserviceFromLocalManifest(doc, "dep-3", "nginx:latest")

	if ms.MemoryLimit == nil {
		t.Fatal("expected memory limit to be set")
	}
	want := int64(512 * 1024 * 1024)
	if *ms.MemoryLimit != want {
		t.Fatalf("MemoryLimit=%d want %d", *ms.MemoryLimit, want)
	}
}
