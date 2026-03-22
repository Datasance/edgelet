package localapi

import (
	"net/http"
	"strings"

	"github.com/eclipse-iofog/agent/internal/auth"
	"github.com/eclipse-iofog/agent/internal/utils/logging"
)

const (
	middlewareModuleName = "Local API Middleware"
)

// authMiddleware validates the access token from the request
func authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract token from Authorization header (matching Java: request.headers().get(HttpHeaderNames.AUTHORIZATION, ""))
		// Java doesn't use "Bearer " prefix - it's just the token directly
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			// Try to get from query parameter as fallback
			authHeader = r.URL.Query().Get("token")
		}

		// Java doesn't use "Bearer " prefix, but some clients might send it
		// Remove "Bearer " prefix if present for compatibility
		token := strings.TrimPrefix(authHeader, "Bearer ")
		token = strings.TrimSpace(token)

		if token == "" {
			logging.LogError(middlewareModuleName, "Missing access token", nil)
			http.Error(w, "Incorrect access token", http.StatusUnauthorized)
			return
		}

		// Validate token
		tokenManager := auth.GetLocalTokenManager()
		if !tokenManager.ValidateToken(token) {
			logging.LogError(middlewareModuleName, "Invalid access token", nil)
			http.Error(w, "Incorrect access token", http.StatusUnauthorized)
			return
		}

		// Token is valid, proceed to next handler
		next(w, r)
	}
}

// loggingMiddleware logs HTTP requests
func loggingMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logging.LogDebug(middlewareModuleName, r.Method+" "+r.URL.Path)
		next(w, r)
	}
}

// chainMiddleware chains multiple middleware functions
func chainMiddleware(handler http.HandlerFunc, middlewares ...func(http.HandlerFunc) http.HandlerFunc) http.HandlerFunc {
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}
	return handler
}
