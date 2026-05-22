package localapi

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
		"iofog.org": map[string]interface{}{
			"rbac": map[string]interface{}{
				"version": "v1",
				"rulesByGroup": map[string]interface{}{
					"agent.datasance.com/v3": []interface{}{
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
		APIGroups: []string{"agent.datasance.com/v3", "agent.iofog.org/v3"},
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
		"iofog.org": map[string]interface{}{
			"rbac": map[string]interface{}{
				"version": "v1",
				"rulesByGroup": map[string]interface{}{
					"agent.datasance.com/v3": []interface{}{
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
		APIGroups: []string{"agent.datasance.com/v3"},
		Resource:  "system/config",
		Verb:      "update",
	}
	if !isAuthorized(claims, perm) {
		t.Fatalf("expected patch->update alias to authorize")
	}
}

func TestMapRequestToPermission_SystemSwitchAndCert(t *testing.T) {
	tests := []struct {
		method   string
		path     string
		resource string
		verb     string
	}{
		{method: http.MethodPost, path: "/v3/system/controller/cert", resource: "system/controller/cert", verb: "update"},
		{method: http.MethodPost, path: "/v3/system/config/switch", resource: "system/config/switch", verb: "update"},
		{method: http.MethodGet, path: "/v3/system/logs", resource: "system/logs", verb: "get"},
		{method: http.MethodGet, path: "/v3/system/logs:stream", resource: "system/logs/stream", verb: "get"},
		{method: http.MethodGet, path: "/v3/images", resource: "images", verb: "get"},
		{method: http.MethodPost, path: "/v3/images:pull", resource: "images/pull", verb: "create"},
		{method: http.MethodGet, path: "/v3/images:pull/abc", resource: "images/pull/status", verb: "get"},
		{method: http.MethodPost, path: "/v3/images:load", resource: "images/load", verb: "create"},
		{method: http.MethodPost, path: "/v3/images:prune", resource: "images/prune", verb: "create"},
		{method: http.MethodPost, path: "/v3/images:remove", resource: "images/remove", verb: "create"},
		{method: http.MethodGet, path: "/v3/deploy/microservices:apply/op-123", resource: "deploy/microservices/apply/status", verb: "get"},
		{method: http.MethodPost, path: "/v3/deploy/runtimeclasses:apply", resource: "deploy/runtimeclasses", verb: "create"},
		{method: http.MethodPost, path: "/v3/deploy/runtimeclasses:validate", resource: "deploy/runtimeclasses", verb: "create"},
		{method: http.MethodGet, path: "/v3/deploy/runtimeclasses", resource: "deploy/runtimeclasses", verb: "get"},
		{method: http.MethodGet, path: "/v3/deploy/runtimeclasses/edgelet", resource: "deploy/runtimeclasses", verb: "get"},
		{method: http.MethodDelete, path: "/v3/deploy/runtimeclasses/edgelet", resource: "deploy/runtimeclasses", verb: "delete"},
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
