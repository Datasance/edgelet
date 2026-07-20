package websocket

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func newTestControlHandler(t *testing.T) (*ControlHandler, *Manager) {
	t.Helper()
	m := NewManager()
	return NewControlHandlerWithManager(m), m
}

func waitForControlConnectionsDrained(t *testing.T, m *Manager) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if m.GetControlConnectionsCount() == 0 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("timed out waiting for control websocket connections to drain")
}

func serviceAccountControlClaims(microserviceUUID string) jwt.MapClaims {
	return jwt.MapClaims{
		"sub":      "system:serviceaccount:app:svc",
		"tokenUse": "serviceaccount",
		"edgelet.iofog.org": map[string]any{
			"microservice": map[string]any{
				"uuid": microserviceUUID,
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
	}
}
