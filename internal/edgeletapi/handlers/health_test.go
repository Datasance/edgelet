package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealthReadyWhenListenerNotReady(t *testing.T) {
	SetEdgeletAPIStartupState(EdgeletAPIStartupInitializing, "booting")
	defer SetEdgeletAPIStartupState(EdgeletAPIStartupInitializing, "local_api_initializing")

	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	w := httptest.NewRecorder()
	HealthReadyHandler(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "local_api_listener_not_ready") {
		t.Fatalf("expected listener-not-ready reason, got body=%s", body)
	}
}

func TestHealthReadyWhenStartupFailed(t *testing.T) {
	SetEdgeletAPIStartupState(EdgeletAPIStartupFailed, "bind failed")
	defer SetEdgeletAPIStartupState(EdgeletAPIStartupInitializing, "local_api_initializing")

	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	w := httptest.NewRecorder()
	HealthReadyHandler(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "local_api_start_failed") {
		t.Fatalf("expected startup-failed reason, got body=%s", body)
	}
	if !strings.Contains(body, "bind failed") {
		t.Fatalf("expected failure detail in body, got=%s", body)
	}
}

func TestHealthLiveIncludesStartupPhase(t *testing.T) {
	SetEdgeletAPIStartupState(EdgeletAPIStartupListening, "")
	defer SetEdgeletAPIStartupState(EdgeletAPIStartupInitializing, "local_api_initializing")

	req := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	w := httptest.NewRecorder()
	HealthLiveHandler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "\"localApiPhase\":\"listening\"") {
		t.Fatalf("expected localApiPhase in response, got body=%s", body)
	}
}
