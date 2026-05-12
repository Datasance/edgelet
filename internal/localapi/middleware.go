package localapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/eclipse-iofog/agent/internal/auth"
	"github.com/golang-jwt/jwt/v5"
)

const (
	middlewareModuleName = "Local API Middleware"
)

var (
	validateLocalJWTFn       = auth.ValidateLocalJWT
	mapRequestToPermissionFn = mapRequestToPermission
	isAuthorizedFn           = isAuthorized
)

// authMiddlewareV3 validates LocalAPI v3 JWTs.
// In unprovisioned mode, unsigned bootstrap JWTs are accepted.
// In provisioned mode, unsigned JWTs are rejected and signed JWTs are required.
func authMiddlewareV3(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			emitRejectEvent(r, routeFromContextOrPath(r), reasonMissingBearerPrefix, http.StatusUnauthorized, nil)
			writeV3Error(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing bearer token")
			return
		}
		token := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))

		if token == "" {
			emitRejectEvent(r, routeFromContextOrPath(r), reasonEmptyToken, http.StatusUnauthorized, nil)
			writeV3Error(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing JWT token")
			return
		}

		result, err := validateLocalJWTFn(token)
		if err != nil {
			emitRejectEvent(
				r,
				routeFromContextOrPath(r),
				mapJWTValidationReasonCode(err),
				http.StatusUnauthorized,
				safeTokenMeta(token),
			)
			writeV3Error(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid JWT token")
			return
		}
		permission, mapped := mapRequestToPermissionFn(r)
		if !mapped {
			emitRejectEvent(r, routeFromContextOrPath(r), reasonRBACUnmappedRoute, http.StatusForbidden, safeClaimsMeta(result.Claims))
			writeV3Error(w, http.StatusForbidden, "FORBIDDEN", "request route is not authorized")
			return
		}
		if !isAuthorizedFn(result.Claims, permission) {
			emitRejectEvent(r, routeFromContextOrPath(r), reasonRBACDenied, http.StatusForbidden, safeClaimsMeta(result.Claims))
			writeV3Error(w, http.StatusForbidden, "FORBIDDEN", "request is not allowed by RBAC policy")
			return
		}

		next(w, withAuthMeta(r, safeClaimsMeta(result.Claims)))
	}
}

func writeV3Error(w http.ResponseWriter, status int, code, message string) {
	body, _ := json.Marshal(map[string]interface{}{
		"success": false,
		"error": map[string]interface{}{
			"code":    code,
			"message": message,
		},
	})
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func safeTokenMeta(token string) map[string]interface{} {
	parsed, _, err := jwt.NewParser().ParseUnverified(token, jwt.MapClaims{})
	if err != nil {
		return map[string]interface{}{}
	}
	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return map[string]interface{}{}
	}
	return safeClaimsMeta(claims)
}

func safeClaimsMeta(claims jwt.MapClaims) map[string]interface{} {
	result := map[string]interface{}{}
	if tokenUse, _ := claims["tokenUse"].(string); strings.TrimSpace(tokenUse) != "" {
		result["tokenUse"] = tokenUse
	}
	if sub, _ := claims["sub"].(string); strings.TrimSpace(sub) != "" {
		result["sub"] = sub
	}
	if jti, _ := claims["jti"].(string); strings.TrimSpace(jti) != "" {
		result["jtiHash"] = hashJTI(jti)
	}
	return result
}

func mapJWTValidationReasonCode(err error) string {
	if err == nil {
		return reasonTokenValidation
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "invalid issuer"):
		return reasonIssuerMismatch
	case strings.Contains(msg, "invalid audience"), strings.Contains(msg, "missing audience"):
		return reasonAudienceMismatch
	case strings.Contains(msg, "invalid token use"):
		return reasonTokenUseMismatch
	case strings.Contains(msg, "unsigned jwt not allowed"), strings.Contains(msg, "only unsigned bootstrap jwt is allowed"):
		return reasonUnsignedRejected
	case strings.Contains(msg, "signed jwt validation failed"):
		return reasonSignatureRequired
	default:
		return reasonTokenValidation
	}
}

// chainMiddleware chains multiple middleware functions
func chainMiddleware(handler http.HandlerFunc, middlewares ...func(http.HandlerFunc) http.HandlerFunc) http.HandlerFunc {
	for _, middleware := range middlewares {
		handler = middleware(handler)
	}
	return handler
}
