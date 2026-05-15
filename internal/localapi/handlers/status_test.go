package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/eclipse-iofog/agent/internal/buildmeta"
)

func TestAugmentWithDNSStatusAddsKeys(t *testing.T) {
	m := map[string]string{
		"iofogDaemon": "running",
	}
	augmentWithDNSStatus(m)

	requiredKeys := []string{
		"dnsStarted",
		"dnsCompatAliasesEnabled",
		"dnsRateLimitEnabled",
		"dnsRateLimitRPS",
		"dnsRateLimitBurst",
		"dnsMaxRequestBytes",
		"dnsMaxQNameBytes",
		"dnsScopeManagedListening",
		"dnsScopeManagedAddress",
		"dnsScopeLocalListening",
		"dnsScopeLocalAddress",
		"dnsQueriesTotal",
		"dnsSuccessTotal",
		"dnsNXDomainTotal",
		"dnsServFailTotal",
		"dnsPolicyDeniedTotal",
		"dnsInactiveTotal",
		"dnsForwardedTotal",
		"dnsForwardErrTotal",
		"dnsForwardingDegraded",
		"dnsForwardTotalUpstream",
		"dnsForwardHealthyUpstream",
		"dnsForwardLastSuccessUnix",
		"dnsForwardLastFailureUnix",
		"dnsForwardBackoffSkipTotal",
		"dnsRateLimitedTotal",
		"dnsRejectedTotal",
		"dnsHealth",
	}
	for _, key := range requiredKeys {
		if _, ok := m[key]; !ok {
			t.Fatalf("missing expected key %q", key)
		}
	}
}

func TestHandleStatus_ExcludesDNSKeysForLiteFlavor(t *testing.T) {
	originalFlavor := buildmeta.Flavor
	buildmeta.Flavor = buildmeta.FlavorLite
	defer func() {
		buildmeta.Flavor = originalFlavor
	}()

	handler := &StatusHandler{}
	req := httptest.NewRequest(http.MethodGet, "/v3/system/status", nil)
	rec := httptest.NewRecorder()
	handler.HandleStatus(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode status payload: %v", err)
	}
	if _, ok := payload["dnsStarted"]; ok {
		t.Fatalf("expected dnsStarted to be absent for lite flavor, payload=%v", payload)
	}
}

func TestHandleStatus_IncludesDNSKeysForFullFlavor(t *testing.T) {
	originalFlavor := buildmeta.Flavor
	buildmeta.Flavor = buildmeta.FlavorFull
	defer func() {
		buildmeta.Flavor = originalFlavor
	}()

	handler := &StatusHandler{}
	req := httptest.NewRequest(http.MethodGet, "/v3/system/status", nil)
	rec := httptest.NewRecorder()
	handler.HandleStatus(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode status payload: %v", err)
	}
	if _, ok := payload["dnsStarted"]; !ok {
		t.Fatalf("expected dnsStarted to be present for full flavor, payload=%v", payload)
	}
}

func TestHandleStatus_IncludesAvailableNetworkInterfaces(t *testing.T) {
	originalFlavor := buildmeta.Flavor
	buildmeta.Flavor = buildmeta.FlavorLite
	defer func() {
		buildmeta.Flavor = originalFlavor
	}()

	handler := &StatusHandler{}
	req := httptest.NewRequest(http.MethodGet, "/v3/system/status", nil)
	rec := httptest.NewRecorder()
	handler.HandleStatus(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode status payload: %v", err)
	}
	if _, ok := payload["availableNetworkInterfaces"]; !ok {
		t.Fatalf("expected availableNetworkInterfaces key in status payload, payload=%v", payload)
	}
}
