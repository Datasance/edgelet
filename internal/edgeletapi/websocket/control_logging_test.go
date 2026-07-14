package websocket

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/eclipse-iofog/edgelet/internal/auth"
	"github.com/golang-jwt/jwt/v5"
	gws "github.com/gorilla/websocket"
)

func waitForControlConnectionsDrained(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if GetManager().GetControlConnectionsCount() == 0 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("timed out waiting for control websocket connections to drain")
}

func TestHandle_UnauthorizedHandshakeEmitsReasonCode(t *testing.T) {
	originalSink := wsLogSink
	defer func() { wsLogSink = originalSink }()
	var captured map[string]any
	wsLogSink = func(_ string, fields map[string]any) {
		if fields["event"] == "edgeletapi.reject" {
			captured = fields
		}
	}

	handler := NewControlHandler()
	req := httptest.NewRequest(http.MethodGet, "/v1/microservices/control", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	rr := httptest.NewRecorder()
	handler.Handle(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
	if captured == nil || captured["reasonCode"] != "websocket_unauthorized" {
		t.Fatalf("expected websocket_unauthorized reject event, got %#v", captured)
	}
	if captured["requestId"] == "" {
		t.Fatal("requestId missing in websocket reject log")
	}
}

func TestHandle_NonGETRejected(t *testing.T) {
	originalSink := wsLogSink
	defer func() { wsLogSink = originalSink }()
	var captured map[string]any
	wsLogSink = func(_ string, fields map[string]any) {
		if fields["event"] == "edgeletapi.reject" {
			captured = fields
		}
	}

	handler := NewControlHandler()
	req := httptest.NewRequest(http.MethodPost, "/v1/microservices/control", nil)
	req.Header.Set("Authorization", "Bearer token")
	rr := httptest.NewRecorder()
	handler.Handle(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rr.Code)
	}
	if captured == nil || captured["reasonCode"] != "method_not_allowed" {
		t.Fatalf("expected method_not_allowed reject event, got %#v", captured)
	}
}

func TestHandle_V3MissingMicroserviceUUIDRejected(t *testing.T) {
	originalSink := wsLogSink
	originalValidate := validateLocalJWTFn
	defer func() {
		wsLogSink = originalSink
		validateLocalJWTFn = originalValidate
	}()
	var captured map[string]any
	wsLogSink = func(_ string, fields map[string]any) {
		if fields["event"] == "edgeletapi.reject" {
			captured = fields
		}
	}
	validateLocalJWTFn = func(string) (*auth.LocalJWTValidationResult, error) {
		return &auth.LocalJWTValidationResult{
			Claims: jwt.MapClaims{
				"sub":      "system:serviceaccount:app:svc",
				"tokenUse": "serviceaccount",
			},
		}, nil
	}

	handler := NewControlHandler()
	req := httptest.NewRequest(http.MethodGet, "/v1/microservices/control", nil)
	req.Header.Set("Authorization", "Bearer mock")
	rr := httptest.NewRecorder()
	handler.Handle(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}
	if captured == nil || captured["reasonCode"] != "websocket_unauthorized" {
		t.Fatalf("expected websocket_unauthorized reject event, got %#v", captured)
	}
}

func TestHandle_V3RBACDeniedRejected(t *testing.T) {
	originalSink := wsLogSink
	originalValidate := validateLocalJWTFn
	originalAuthorize := authorizeV3WSFn
	defer func() {
		wsLogSink = originalSink
		validateLocalJWTFn = originalValidate
		authorizeV3WSFn = originalAuthorize
	}()
	var captured map[string]any
	wsLogSink = func(_ string, fields map[string]any) {
		if fields["event"] == "edgeletapi.reject" {
			captured = fields
		}
	}
	validateLocalJWTFn = func(string) (*auth.LocalJWTValidationResult, error) {
		return &auth.LocalJWTValidationResult{
			Claims: jwt.MapClaims{
				"sub":      "system:serviceaccount:app:svc",
				"tokenUse": "serviceaccount",
				"edgelet.iofog.org": map[string]any{
					"microservice": map[string]any{
						"uuid": "ms-1",
					},
				},
			},
		}, nil
	}
	authorizeV3WSFn = func(jwt.MapClaims) bool { return false }

	handler := NewControlHandler()
	req := httptest.NewRequest(http.MethodGet, "/v1/microservices/control", nil)
	req.Header.Set("Authorization", "Bearer mock")
	rr := httptest.NewRecorder()
	handler.Handle(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}
	if captured == nil || captured["reasonCode"] != "rbac_denied" {
		t.Fatalf("expected rbac_denied reject event, got %#v", captured)
	}
}

func TestHandle_UpgradeFailureEmitsReasonCode(t *testing.T) {
	originalSink := wsLogSink
	originalValidate := validateLocalJWTFn
	originalAuthorize := authorizeV3WSFn
	defer func() {
		wsLogSink = originalSink
		validateLocalJWTFn = originalValidate
		authorizeV3WSFn = originalAuthorize
	}()
	var captured map[string]any
	wsLogSink = func(_ string, fields map[string]any) {
		if fields["event"] == "edgeletapi.reject" && fields["reasonCode"] == "websocket_upgrade_failed" {
			captured = fields
		}
	}
	validateLocalJWTFn = func(string) (*auth.LocalJWTValidationResult, error) {
		return &auth.LocalJWTValidationResult{
			Claims: jwt.MapClaims{
				"sub":      "system:serviceaccount:app:svc",
				"tokenUse": "serviceaccount",
				"edgelet.iofog.org": map[string]any{
					"microservice": map[string]any{
						"uuid": "ms-1",
					},
				},
			},
		}, nil
	}
	authorizeV3WSFn = func(jwt.MapClaims) bool { return true }

	handler := NewControlHandler()
	req := httptest.NewRequest(http.MethodGet, "/v1/microservices/control", nil)
	req.Header.Set("Authorization", "Bearer mock")
	rr := httptest.NewRecorder()
	handler.Handle(rr, req)

	if captured == nil || captured["reasonCode"] != "websocket_upgrade_failed" {
		t.Fatalf("expected websocket_upgrade_failed event, got %#v", captured)
	}
}

func TestHandle_V3UpgradeRegression(t *testing.T) {
	originalValidate := validateLocalJWTFn
	originalAuthorize := authorizeV3WSFn
	defer func() {
		validateLocalJWTFn = originalValidate
		authorizeV3WSFn = originalAuthorize
	}()
	validateLocalJWTFn = func(string) (*auth.LocalJWTValidationResult, error) {
		return &auth.LocalJWTValidationResult{
			Claims: jwt.MapClaims{
				"sub":      "system:serviceaccount:app:svc",
				"tokenUse": "serviceaccount",
				"edgelet.iofog.org": map[string]any{
					"microservice": map[string]any{
						"uuid": "ms-1",
					},
					"rbac": map[string]any{
						"rulesByGroup": map[string]any{
							"edgelet.iofog.org/v1": []any{
								map[string]any{
									"resources": []any{"microservices/control/self"},
									"verbs":     []any{"get"},
								},
							},
						},
					},
				},
			},
		}, nil
	}
	authorizeV3WSFn = func(jwt.MapClaims) bool { return true }

	handler := NewControlHandler()
	server := httptest.NewServer(http.HandlerFunc(handler.Handle))
	defer func() {
		server.Close()
	}()

	dial := func(path string) error {
		wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + path
		h := http.Header{}
		h.Set("Authorization", "Bearer mock")
		conn, _, err := gws.DefaultDialer.Dial(wsURL, h)
		if err != nil {
			return err
		}
		return conn.Close()
	}

	if err := dial("/v1/microservices/control"); err != nil {
		t.Fatalf("expected v3 websocket upgrade success, got err=%v", err)
	}
	waitForControlConnectionsDrained(t)
}
