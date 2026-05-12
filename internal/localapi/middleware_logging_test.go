package localapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eclipse-iofog/agent/internal/auth"
	"github.com/golang-jwt/jwt/v5"
)

func TestRequestIDMiddleware_GeneratesWhenMissing(t *testing.T) {
	handler := requestIdMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if requestIDFromContext(r.Context()) == "" {
			t.Fatalf("requestId missing from context")
		}
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodGet, "/v3/system/status", nil)
	rr := httptest.NewRecorder()
	handler(rr, req)
	if rr.Header().Get(requestIDHeader) == "" {
		t.Fatalf("missing %s header", requestIDHeader)
	}
}

func TestRequestIDMiddleware_PassthroughWhenProvided(t *testing.T) {
	handler := requestIdMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if got := requestIDFromContext(r.Context()); got != "req-123" {
			t.Fatalf("unexpected requestId %q", got)
		}
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodGet, "/v3/system/status", nil)
	req.Header.Set(requestIDHeader, "req-123")
	rr := httptest.NewRecorder()
	handler(rr, req)
	if rr.Header().Get(requestIDHeader) != "req-123" {
		t.Fatalf("expected passthrough requestId header")
	}
}

func TestAccessLoggingMiddleware_EmitsStructuredAccessFields(t *testing.T) {
	var captured structuredEvent
	originalSink := localAPILogSink
	localAPILogSink = func(event structuredEvent) { captured = event }
	defer func() { localAPILogSink = originalSink }()

	handler := requestIdMiddleware(accessLoggingMiddleware(withRoute("/v3/system/status", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("ok"))
	})))

	req := httptest.NewRequest(http.MethodGet, "/v3/system/status", nil)
	req.Host = "unix"
	req.RemoteAddr = "@/run/iofog-agent/iofog-agentd.sock"
	rr := httptest.NewRecorder()
	handler(rr, req)

	if captured.Fields["event"] != "localapi.access" {
		t.Fatalf("unexpected event: %#v", captured.Fields["event"])
	}
	if captured.Fields["transport"] != "unix" || captured.Fields["scheme"] != "http+unix" {
		t.Fatalf("unexpected transport/scheme: %#v %#v", captured.Fields["transport"], captured.Fields["scheme"])
	}
	if captured.Fields["status"] != http.StatusCreated {
		t.Fatalf("unexpected status: %#v", captured.Fields["status"])
	}
	if captured.Fields["requestId"] == "" {
		t.Fatalf("requestId missing from access event")
	}
}

func TestAuthMiddlewareV3_MissingBearerPrefixEmitsReasonCode(t *testing.T) {
	var captured structuredEvent
	originalSink := localAPILogSink
	localAPILogSink = func(event structuredEvent) { captured = event }
	defer func() { localAPILogSink = originalSink }()

	handler := requestIdMiddleware(accessLoggingMiddleware(authMiddlewareV3(withRoute("/v3/system/status", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))))
	req := httptest.NewRequest(http.MethodGet, "/v3/system/status", nil)
	rr := httptest.NewRecorder()
	handler(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
	if captured.Fields["event"] != "localapi.access" {
		t.Fatalf("last event should be access completion")
	}
}

func TestAuthMiddlewareV3_UnmappedRouteStrictDenyAndReasonCode(t *testing.T) {
	var rejectEvent structuredEvent
	originalSink := localAPILogSink
	localAPILogSink = func(event structuredEvent) {
		if event.Fields["event"] == "localapi.reject" {
			rejectEvent = event
		}
	}
	defer func() { localAPILogSink = originalSink }()

	origValidate := validateLocalJWTFn
	origMap := mapRequestToPermissionFn
	defer func() {
		validateLocalJWTFn = origValidate
		mapRequestToPermissionFn = origMap
	}()
	validateLocalJWTFn = func(string) (*auth.LocalJWTValidationResult, error) {
		return &auth.LocalJWTValidationResult{Claims: jwt.MapClaims{"sub": "system:serviceaccount:app:ms", "tokenUse": "serviceaccount", "jti": "jti-1"}}, nil
	}
	mapRequestToPermissionFn = func(*http.Request) (rbacPermission, bool) {
		return rbacPermission{}, false
	}

	handler := requestIdMiddleware(accessLoggingMiddleware(authMiddlewareV3(withRoute("/v3/unmapped", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))))
	req := httptest.NewRequest(http.MethodGet, "/v3/unmapped", nil)
	req.Header.Set("Authorization", "Bearer token")
	rr := httptest.NewRecorder()
	handler(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}
	if rejectEvent.Fields["reasonCode"] != reasonRBACUnmappedRoute {
		t.Fatalf("expected %s, got %#v", reasonRBACUnmappedRoute, rejectEvent.Fields["reasonCode"])
	}
}

func TestAuthMiddlewareV3_RBACDeniedReasonCode(t *testing.T) {
	var rejectEvent structuredEvent
	originalSink := localAPILogSink
	localAPILogSink = func(event structuredEvent) {
		if event.Fields["event"] == "localapi.reject" {
			rejectEvent = event
		}
	}
	defer func() { localAPILogSink = originalSink }()

	origValidate := validateLocalJWTFn
	origMap := mapRequestToPermissionFn
	origAuthorize := isAuthorizedFn
	defer func() {
		validateLocalJWTFn = origValidate
		mapRequestToPermissionFn = origMap
		isAuthorizedFn = origAuthorize
	}()
	validateLocalJWTFn = func(string) (*auth.LocalJWTValidationResult, error) {
		return &auth.LocalJWTValidationResult{Claims: jwt.MapClaims{"sub": "system:serviceaccount:app:ms", "tokenUse": "serviceaccount", "jti": "jti-2"}}, nil
	}
	mapRequestToPermissionFn = func(*http.Request) (rbacPermission, bool) {
		return rbacPermission{Resource: "system/config", Verb: "get"}, true
	}
	isAuthorizedFn = func(jwt.MapClaims, rbacPermission) bool { return false }

	handler := requestIdMiddleware(accessLoggingMiddleware(authMiddlewareV3(withRoute("/v3/system/config", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))))
	req := httptest.NewRequest(http.MethodGet, "/v3/system/config", nil)
	req.Header.Set("Authorization", "Bearer token")
	rr := httptest.NewRecorder()
	handler(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}
	if rejectEvent.Fields["reasonCode"] != reasonRBACDenied {
		t.Fatalf("expected %s, got %#v", reasonRBACDenied, rejectEvent.Fields["reasonCode"])
	}
}

func TestAuthMiddlewareV3_TokenIsRedactedFromRejectLogs(t *testing.T) {
	var rejectEvent structuredEvent
	originalSink := localAPILogSink
	localAPILogSink = func(event structuredEvent) {
		if event.Fields["event"] == "localapi.reject" {
			rejectEvent = event
		}
	}
	defer func() { localAPILogSink = originalSink }()

	origValidate := validateLocalJWTFn
	defer func() { validateLocalJWTFn = origValidate }()
	validateLocalJWTFn = func(string) (*auth.LocalJWTValidationResult, error) {
		return nil, errors.New("boom")
	}

	rawToken := "secret-token-value"
	handler := requestIdMiddleware(accessLoggingMiddleware(authMiddlewareV3(withRoute("/v3/system/status", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))))
	req := httptest.NewRequest(http.MethodGet, "/v3/system/status", nil)
	req.Header.Set("Authorization", "Bearer "+rawToken)
	rr := httptest.NewRecorder()
	handler(rr, req)

	encoded, _ := json.Marshal(rejectEvent.Fields)
	if strings.Contains(string(encoded), rawToken) {
		t.Fatalf("raw token leaked in logs: %s", encoded)
	}
	if _, exists := rejectEvent.Fields["jtiHash"]; exists {
		// parseUnverified fails for raw token string; this field should not appear.
		t.Fatalf("unexpected jtiHash for non-JWT token")
	}
}
