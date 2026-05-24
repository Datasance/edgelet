package auth

import (
	"net/http"
	"strings"

	"github.com/datasance/edgelet/internal/utils/logging"
)

const (
	middlewareModuleName = "Auth Middleware"
)

// LocalAPIAuthMiddleware creates middleware for validating local API tokens
func LocalAPIAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Get token from Authorization header
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			logging.LogDebug(middlewareModuleName, "Missing Authorization header")
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Remove "Bearer " prefix if present (though Java version doesn't use it)
		token := strings.TrimPrefix(authHeader, "Bearer ")
		token = strings.TrimSpace(token)

		// Validate token
		tokenManager := GetLocalTokenManager()
		if !tokenManager.ValidateToken(token) {
			logging.LogDebug(middlewareModuleName, "Invalid access token")
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Token is valid, proceed to next handler
		next.ServeHTTP(w, r)
	})
}

// ValidateAccessToken is a helper function to validate access tokens (for compatibility with Java API)
func ValidateAccessToken(token string) bool {
	tokenManager := GetLocalTokenManager()
	return tokenManager.ValidateToken(token)
}
