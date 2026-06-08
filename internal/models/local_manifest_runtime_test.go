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
