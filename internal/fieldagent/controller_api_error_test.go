package fieldagent

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eclipse-iofog/edgelet/internal/auth"
	"github.com/eclipse-iofog/edgelet/internal/config"
	"github.com/eclipse-iofog/edgelet/internal/models"
	"github.com/eclipse-iofog/edgelet/internal/statusreporter"
)

const (
	sampleAgent401     = `{"error":"Unauthorized","code":"AGENT_JWT_SIGNATURE_INVALID","message":"Agent JWT signature invalid","retryable":false}`
	sampleAgent503     = `{"error":"ServiceUnavailable","code":"CONTROLLER_DB_BUSY","message":"Database temporarily unavailable","retryable":true}`
	sampleReadiness503 = `{"status":"online","timestamp":1710000000123,"uptimeSec":42.5,"versions":{"controller":"3.8.2"},"error":"ServiceUnavailable","code":"CONTROLLER_NOT_READY","message":"Authentication subsystem is not ready","retryable":true}`
	sampleLegacy401    = `{"name":"AuthenticationError","message":"Expired provision key"}`
)

func TestParseControllerAPIError_Agent401(t *testing.T) {
	err := ParseControllerAPIError(http.StatusUnauthorized, sampleAgent401)
	var apiErr *ControllerAPIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *ControllerAPIError, got %T", err)
	}
	if apiErr.Code != "AGENT_JWT_SIGNATURE_INVALID" {
		t.Fatalf("unexpected code: %q", apiErr.Code)
	}
	if apiErr.Retryable {
		t.Fatal("expected retryable false")
	}
	if apiErr.Legacy {
		t.Fatal("expected legacy false")
	}
}

func TestParseControllerAPIError_Agent503(t *testing.T) {
	err := ParseControllerAPIError(http.StatusServiceUnavailable, sampleAgent503)
	var apiErr *ControllerAPIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *ControllerAPIError, got %T", err)
	}
	if apiErr.Code != "CONTROLLER_DB_BUSY" {
		t.Fatalf("unexpected code: %q", apiErr.Code)
	}
	if !apiErr.Retryable {
		t.Fatal("expected retryable true")
	}
}

func TestParseControllerAPIError_Readiness503(t *testing.T) {
	err := ParseControllerAPIError(http.StatusServiceUnavailable, sampleReadiness503)
	var apiErr *ControllerAPIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *ControllerAPIError, got %T", err)
	}
	if apiErr.Code != "CONTROLLER_NOT_READY" {
		t.Fatalf("unexpected code: %q", apiErr.Code)
	}
	if !apiErr.Retryable {
		t.Fatal("expected retryable true")
	}
	if apiErr.Message != "Authentication subsystem is not ready" {
		t.Fatalf("unexpected message: %q", apiErr.Message)
	}
}

func TestParseControllerAPIError_Legacy401(t *testing.T) {
	err := ParseControllerAPIError(http.StatusUnauthorized, sampleLegacy401)
	var apiErr *ControllerAPIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *ControllerAPIError, got %T", err)
	}
	if !apiErr.Legacy {
		t.Fatal("expected legacy true")
	}
	if apiErr.Code != "" {
		t.Fatalf("expected empty code, got %q", apiErr.Code)
	}
	if apiErr.Message != "Expired provision key" {
		t.Fatalf("unexpected message: %q", apiErr.Message)
	}
}

func TestParseControllerAPIError_503DefaultsRetryable(t *testing.T) {
	err := ParseControllerAPIError(http.StatusServiceUnavailable, "")
	var apiErr *ControllerAPIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *ControllerAPIError, got %T", err)
	}
	if !apiErr.Retryable {
		t.Fatal("expected retryable true when field missing on 503")
	}
}

func TestShouldRetry_ControllerAPIErrors(t *testing.T) {
	if !shouldRetry(ParseControllerAPIError(http.StatusServiceUnavailable, sampleAgent503)) {
		t.Fatal("expected retry on 503")
	}
	if shouldRetry(ParseControllerAPIError(http.StatusUnauthorized, sampleAgent401)) {
		t.Fatal("expected no retry on structured 401")
	}
	if !shouldRetry(ParseControllerAPIError(http.StatusUnauthorized, sampleLegacy401)) {
		t.Fatal("expected retry on legacy 401")
	}
}

func TestIsControllerNotReady(t *testing.T) {
	if !IsControllerNotReady(ParseControllerAPIError(http.StatusServiceUnavailable, sampleReadiness503)) {
		t.Fatal("expected readiness 503 to be not ready")
	}
	if IsControllerNotReady(ParseControllerAPIError(http.StatusUnauthorized, sampleAgent401)) {
		t.Fatal("expected 401 not to be not-ready")
	}
}

func TestAPIClient_Ping_Readiness503(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(sampleReadiness503))
	}))
	t.Cleanup(srv.Close)

	client := &APIClient{
		baseURL:    srv.URL,
		httpClient: srv.Client(),
	}

	ok, err := client.Ping(context.Background())
	if ok {
		t.Fatal("expected ping ok=false on 503")
	}
	if err == nil {
		t.Fatal("expected error on 503")
	}
	if !IsControllerNotReady(err) {
		t.Fatalf("expected controller not ready error, got: %v", err)
	}
	var apiErr *ControllerAPIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *ControllerAPIError, got %T", err)
	}
	if apiErr.Code != "CONTROLLER_NOT_READY" {
		t.Fatalf("unexpected code: %q", apiErr.Code)
	}
}

func TestPingWithTransition_Readiness503_NoVerificationFailed(t *testing.T) {
	openFieldAgentTestDB(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/status") {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(sampleReadiness503))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	cfg := config.GetInstance()
	origURL := cfg.ControllerURL
	origUUID := cfg.IOFogUUID
	origKey := cfg.PrivateKey
	cfg.ControllerURL = srv.URL
	cfg.IOFogUUID = "uuid-ping-503"
	cfg.PrivateKey = testProvisionPrivateKeyBase64(t)
	t.Cleanup(func() {
		cfg.ControllerURL = origURL
		cfg.IOFogUUID = origUUID
		cfg.PrivateKey = origKey
	})

	fa := &FieldAgent{
		config: cfg,
		state:  NewState(),
		ctx:    context.Background(),
		apiClient: &APIClient{
			baseURL:    srv.URL,
			httpClient: srv.Client(),
			jwtManager: auth.GetJWTManager(),
		},
	}
	fa.state.SetControllerStatus(models.ControllerStatusOK)
	fa.state.SetControllerVerified(true)
	statusreporter.GetInstance().UpdateFieldAgentStatus(func(status *models.FieldAgentStatus) {
		status.ControllerStatus = models.ControllerStatusOK
		status.ControllerVerified = true
	})

	ok, transitioned := fa.pingWithTransition()
	if ok || transitioned {
		t.Fatalf("expected ping failure without transition, got ok=%v transitioned=%v", ok, transitioned)
	}
	if fa.state.GetControllerStatus() != models.ControllerStatusNotConnected {
		t.Fatalf("expected NOT_CONNECTED, got %s", fa.state.GetControllerStatus())
	}
	if fa.state.IsControllerVerified() {
		t.Fatal("expected ControllerVerified=false")
	}
	status := statusreporter.GetInstance().GetFieldAgentStatus()
	if status.ControllerVerified {
		t.Fatal("expected status reporter ControllerVerified=false")
	}
	if status.ControllerStatus != models.ControllerStatusNotConnected {
		t.Fatalf("expected status reporter NOT_CONNECTED, got %s", status.ControllerStatus)
	}
}

func TestAPIClient_DoRequest_Structured401(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(sampleAgent401))
	}))
	t.Cleanup(srv.Close)

	client := &APIClient{
		baseURL:    srv.URL,
		httpClient: srv.Client(),
		jwtManager: auth.GetJWTManager(),
	}

	_, err := client.doRequest(context.Background(), "status", PUT, nil, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	var apiErr *ControllerAPIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *ControllerAPIError, got %T: %v", err, err)
	}
	if apiErr.Code != "AGENT_JWT_SIGNATURE_INVALID" {
		t.Fatalf("unexpected code: %q", apiErr.Code)
	}
}

func TestAPIClient_Ping_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "online"})
	}))
	t.Cleanup(srv.Close)

	client := &APIClient{
		baseURL:    srv.URL,
		httpClient: srv.Client(),
	}

	ok, err := client.Ping(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected ping ok=true")
	}
}
