package dnsresolver

import (
	"strings"
	"testing"
)

func TestControlPlaneAliasesForWorkload_ResolvesThreeFQDNs(t *testing.T) {
	const (
		namespace = "default"
		name      = "pot"
		ip        = "10.42.0.15"
	)
	r := newTestResolver()
	r.scopeEnabled[ScopeLocal] = true
	r.UpsertWorkload(WorkloadRecord{
		UUID:         "cp-uuid-test",
		Application:  namespace,
		Name:         name,
		Scope:        ScopeLocal,
		IP:           ip,
		IsController: true,
		Active:       true,
	})

	for _, fqdn := range ControlPlaneFQDNs(namespace, name) {
		known, answers, denied := r.resolveInternal(ScopeLocal, fqdn, 1)
		if !known || denied {
			t.Fatalf("expected %q to resolve, known=%v denied=%v", fqdn, known, denied)
		}
		if len(answers) == 0 {
			t.Fatalf("expected answer for %q", fqdn)
		}
		if got := answers[0].String(); !strings.Contains(got, ip) {
			t.Fatalf("expected %q to resolve to %s, got %s", fqdn, ip, got)
		}
	}
}

func TestControlPlaneAliasesForWorkload_IncludesShortAndFQDNForms(t *testing.T) {
	rec := WorkloadRecord{
		UUID:         "cp-1",
		Application:  "default",
		Name:         "pot",
		Scope:        ScopeLocal,
		IsController: true,
	}
	aliases := aliasesForWorkload(rec, false)

	for _, want := range []string{
		"default.pot",
		"default.pot.svc.bridge.local",
		ControlPlaneBridgeAliasEdgeletController(),
		ControlPlaneBridgeAliasEdgeletController() + "." + defaultZoneName,
		ControlPlaneBridgeAliasNamespaceController("default"),
		ControlPlaneBridgeAliasNamespaceController("default") + "." + defaultZoneName,
	} {
		if !containsAlias(aliases, want) {
			t.Fatalf("missing alias %q in %v", want, aliases)
		}
	}
}

func TestWorkloadBridgeNetworkAliases_ControlPlane(t *testing.T) {
	got := WorkloadBridgeNetworkAliases("default", "pot", true)
	if len(got) != 3 {
		t.Fatalf("expected 3 docker aliases, got %v", got)
	}
	if got[0] != "default.pot" || got[1] != "edgelet.controller" || got[2] != "controller.default" {
		t.Fatalf("unexpected alias order/content: %v", got)
	}
}
