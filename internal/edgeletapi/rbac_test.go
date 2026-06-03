package edgeletapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang-jwt/jwt/v5"
)

func TestIsAuthorized_ReadsCanonicalRulesByGroup(t *testing.T) {
	claims := jwt.MapClaims{
		"tokenUse": "serviceaccount",
		"sub":      "system:serviceaccount:app:svc",
		"edgelet.iofog.org": map[string]interface{}{
			"rbac": map[string]interface{}{
				"version": "v1",
				"rulesByGroup": map[string]interface{}{
					"edgelet.iofog.org/v1": []interface{}{
						map[string]interface{}{
							"resources": []interface{}{"system/config"},
							"verbs":     []interface{}{"get"},
						},
					},
				},
			},
		},
	}
	perm := rbacPermission{
		APIGroups: []string{"edgelet.iofog.org/v1", "edgelet.iofog.org/v1"},
		Resource:  "system/config",
		Verb:      "get",
	}
	if !isAuthorized(claims, perm) {
		t.Fatalf("expected canonical rulesByGroup authorization to pass")
	}
}

func TestIsAuthorized_AcceptsVerbAliases(t *testing.T) {
	claims := jwt.MapClaims{
		"tokenUse": "serviceaccount",
		"sub":      "system:serviceaccount:app:svc",
		"edgelet.iofog.org": map[string]interface{}{
			"rbac": map[string]interface{}{
				"version": "v1",
				"rulesByGroup": map[string]interface{}{
					"edgelet.iofog.org/v1": []interface{}{
						map[string]interface{}{
							"resources": []interface{}{"system/config"},
							"verbs":     []interface{}{"patch"},
						},
					},
				},
			},
		},
	}
	perm := rbacPermission{
		APIGroups: []string{"edgelet.iofog.org/v1"},
		Resource:  "system/config",
		Verb:      "update",
	}
	if !isAuthorized(claims, perm) {
		t.Fatalf("expected patch->update alias to authorize")
	}
}

func TestMapRequestToPermission_ControlPlaneRoutes(t *testing.T) {
	tests := []struct {
		method   string
		path     string
		resource string
		verb     string
	}{
		{method: http.MethodGet, path: "/v1/system/controlplane", resource: "system/controlplane", verb: "get"},
		{method: http.MethodGet, path: "/v1/system/controlplane/manifest", resource: "system/controlplane", verb: "get"},
		{method: http.MethodDelete, path: "/v1/system/controlplane", resource: "system/controlplane", verb: "delete"},
		{method: http.MethodGet, path: "/v1/system/controller", resource: "system/controller", verb: "get"},
		{method: http.MethodPost, path: "/v1/deploy/controlplane:apply", resource: "deploy/controlplane", verb: "create"},
		{method: http.MethodGet, path: "/v1/deploy/controlplane:apply/op-123", resource: "deploy/controlplane/apply/status", verb: "get"},
		{method: http.MethodPost, path: "/v1/deploy/controlplane:validate", resource: "deploy/controlplane", verb: "create"},
	}
	for _, tt := range tests {
		req := httptest.NewRequest(tt.method, tt.path, nil)
		perm, ok := mapRequestToPermission(req)
		if !ok {
			t.Fatalf("expected route %s to map", tt.path)
		}
		if perm.Resource != tt.resource || perm.Verb != tt.verb {
			t.Fatalf("unexpected mapping for %s: resource=%s verb=%s", tt.path, perm.Resource, perm.Verb)
		}
	}
}

func TestMapRequestToPermission_SystemSwitchAndCert(t *testing.T) {
	tests := []struct {
		method   string
		path     string
		resource string
		verb     string
	}{
		{method: http.MethodPost, path: "/v1/system/controller/cert", resource: "system/controller/cert", verb: "update"},
		{method: http.MethodPost, path: "/v1/system/config/switch", resource: "system/config/switch", verb: "update"},
		{method: http.MethodGet, path: "/v1/system/logs", resource: "system/logs", verb: "get"},
		{method: http.MethodGet, path: "/v1/system/logs:stream", resource: "system/logs/stream", verb: "get"},
		{method: http.MethodGet, path: "/v1/images", resource: "images", verb: "get"},
		{method: http.MethodPost, path: "/v1/images:pull", resource: "images/pull", verb: "create"},
		{method: http.MethodGet, path: "/v1/images:pull/abc", resource: "images/pull/status", verb: "get"},
		{method: http.MethodPost, path: "/v1/images:load", resource: "images/load", verb: "create"},
		{method: http.MethodGet, path: "/v1/images:load/abc", resource: "images/load/status", verb: "get"},
		{method: http.MethodPost, path: "/v1/images:prune", resource: "images/prune", verb: "create"},
		{method: http.MethodPost, path: "/v1/images:remove", resource: "images/remove", verb: "create"},
		{method: http.MethodGet, path: "/v1/deploy/microservices:apply/op-123", resource: "deploy/microservices/apply/status", verb: "get"},
		{method: http.MethodPost, path: "/v1/deploy/runtimeclasses:apply", resource: "deploy/runtimeclasses", verb: "create"},
		{method: http.MethodPost, path: "/v1/deploy/runtimeclasses:validate", resource: "deploy/runtimeclasses", verb: "create"},
		{method: http.MethodGet, path: "/v1/deploy/runtimeclasses", resource: "deploy/runtimeclasses", verb: "get"},
		{method: http.MethodGet, path: "/v1/deploy/runtimeclasses/edgelet", resource: "deploy/runtimeclasses", verb: "get"},
		{method: http.MethodDelete, path: "/v1/deploy/runtimeclasses/edgelet", resource: "deploy/runtimeclasses", verb: "delete"},
	}
	for _, tt := range tests {
		req := httptest.NewRequest(tt.method, tt.path, nil)
		perm, ok := mapRequestToPermission(req)
		if !ok {
			t.Fatalf("expected route %s to map", tt.path)
		}
		if perm.Resource != tt.resource || perm.Verb != tt.verb {
			t.Fatalf("unexpected mapping for %s: resource=%s verb=%s", tt.path, perm.Resource, perm.Verb)
		}
	}
}
