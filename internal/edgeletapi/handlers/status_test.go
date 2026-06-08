package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/eclipse-iofog/edgelet/internal/buildmeta"
	"github.com/eclipse-iofog/edgelet/internal/config"
	"github.com/eclipse-iofog/edgelet/internal/constants"
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

func TestHandleStatus_ExcludesDNSKeysWithoutEmbeddedEdgeletEngine(t *testing.T) {
	embedded := false
	buildmeta.SetHasEmbeddedEngineForTest(&embedded)
	defer buildmeta.SetHasEmbeddedEngineForTest(nil)

	handler := &StatusHandler{}
	req := httptest.NewRequest(http.MethodGet, "/v1/system/status", nil)
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
		t.Fatalf("expected dnsStarted to be absent without embedded edgelet engine, payload=%v", payload)
	}
}

func TestHandleStatus_IncludesDNSKeysForEmbeddedEdgeletEngine(t *testing.T) {
	embedded := true
	buildmeta.SetHasEmbeddedEngineForTest(&embedded)
	defer buildmeta.SetHasEmbeddedEngineForTest(nil)

	cfg := config.GetInstance()
	originalEngine := cfg.ContainerEngine
	cfg.ContainerEngine = constants.EngineEdgelet
	defer func() { cfg.ContainerEngine = originalEngine }()

	handler := &StatusHandler{}
	req := httptest.NewRequest(http.MethodGet, "/v1/system/status", nil)
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
		t.Fatalf("expected dnsStarted to be present for embedded edgelet engine, payload=%v", payload)
	}
}

func TestHandleStatus_IncludesAvailableNetworkInterfaces(t *testing.T) {
	handler := &StatusHandler{}
	req := httptest.NewRequest(http.MethodGet, "/v1/system/status", nil)
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

func TestHandleStatus_IncludesAvailableRuntimes(t *testing.T) {
	handler := &StatusHandler{}
	req := httptest.NewRequest(http.MethodGet, "/v1/system/status", nil)
	rec := httptest.NewRecorder()
	handler.HandleStatus(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode status payload: %v", err)
	}
	if _, ok := payload["availableRuntimes"]; !ok {
		t.Fatalf("expected availableRuntimes key in status payload, payload=%v", payload)
	}
}
