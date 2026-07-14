package websocket

import "github.com/golang-jwt/jwt/v5"

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
