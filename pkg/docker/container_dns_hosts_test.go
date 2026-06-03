package docker

import (
	"testing"

	"github.com/datasance/edgelet/internal/dnsresolver"
)

func TestBuildExtraHostsWithIoFog_UsesCanonicalAgentHost(t *testing.T) {
	got := buildExtraHostsWithIoFog([]string{"custom.local:10.0.0.10"}, "10.0.0.2")
	if len(got) == 0 || got[0] != canonicalAgentHost+":10.0.0.2" {
		t.Fatalf("expected canonical host mapping to be prepended, got %v", got)
	}

	already := []string{canonicalAgentHost + ":10.0.0.99", "custom.local:10.0.0.10"}
	got = buildExtraHostsWithIoFog(already, "10.0.0.2")
	count := 0
	for _, h := range got {
		if h == canonicalAgentHost+":10.0.0.99" || h == canonicalAgentHost+":10.0.0.2" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected no duplicate canonical agent host entry, got %v", got)
	}
}

func TestAppendCanonicalReservedHosts_AppendsRouterAndNats(t *testing.T) {
	base := []string{"custom.local:10.0.0.10"}
	got := appendCanonicalReservedHosts(base, "10.0.0.3", "10.0.0.4")

	hasRouter := false
	hasNats := false
	for _, h := range got {
		if h == canonicalRouterHost+":10.0.0.3" {
			hasRouter = true
		}
		if h == canonicalNatsHost+":10.0.0.4" {
			hasNats = true
		}
	}
	if !hasRouter || !hasNats {
		t.Fatalf("expected router and nats canonical entries, got %v", got)
	}
}

func TestAppendCanonicalReservedHosts_DeduplicatesHostnames(t *testing.T) {
	base := []string{
		canonicalRouterHost + ":10.0.0.30",
		canonicalNatsHost + ":10.0.0.40",
	}
	got := appendCanonicalReservedHosts(base, "10.0.0.3", "10.0.0.4")

	routerCount := 0
	natsCount := 0
	for _, h := range got {
		if host, ok := hostKey(h); ok && host == canonicalRouterHost {
			routerCount++
		}
		if host, ok := hostKey(h); ok && host == canonicalNatsHost {
			natsCount++
		}
	}
	if routerCount != 1 || natsCount != 1 {
		t.Fatalf("expected deduped canonical host entries, got %v", got)
	}
}

func TestWorkloadBridgeNetworkAliases_ControlPlaneDocker(t *testing.T) {
	got := dnsresolver.WorkloadBridgeNetworkAliases("default", "pot", true)
	want := []string{"default.pot", "edgelet.controller", "controller.default"}
	if len(got) != len(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected %v, got %v", want, got)
		}
	}
}
