package handlers

import (
	"net/http"
	"strings"

	"github.com/datasance/edgelet/internal/auth"
)

// AuthHandler handles auth-related EdgeletAPI requests.
type AuthHandler struct{}

// NewAuthHandler creates a new auth handler.
func NewAuthHandler() *AuthHandler {
	return &AuthHandler{}
}

// AuthWhoAmIResponse returns caller identity details.
type AuthWhoAmIResponse struct {
	Subject   string                 `json:"subject"`
	TokenType string                 `json:"tokenType"`
	Issuer    string                 `json:"issuer"`
	Audience  []string               `json:"audience"`
	JTI       string                 `json:"jti,omitempty"`
	ExpiresAt int64                  `json:"expiresAt,omitempty"`
	Claims    map[string]interface{} `json:"claims"`
}

// HandleWhoAmI handles GET /v1/auth/whoami.
func (h *AuthHandler) HandleWhoAmI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeAPIError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed, "method not allowed", nil)
		return
	}

	authHeader := r.Header.Get("Authorization")
	token := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
	result, err := auth.ValidateEdgeletAPIJWT(token)
	if err != nil {
		writeAPIError(w, http.StatusUnauthorized, ErrCodeUnauthorized, "invalid JWT token", nil)
		return
	}

	resp := AuthWhoAmIResponse{
		TokenType: claimAsString(map[string]interface{}(result.Claims), "tokenUse"),
		Issuer:    claimAsString(result.Claims, "iss"),
		Audience:  claimAsStringSlice(result.Claims, "aud"),
		JTI:       claimAsString(result.Claims, "jti"),
		ExpiresAt: claimAsInt64(result.Claims, "exp"),
		Claims:    map[string]interface{}(result.Claims),
	}
	resp.Subject = claimAsString(result.Claims, "sub")

	writeSuccess(w, http.StatusOK, resp)
}

func claimAsString(claims map[string]interface{}, key string) string {
	value, exists := claims[key]
	if !exists || value == nil {
		return ""
	}
	if s, ok := value.(string); ok {
		return s
	}
	return ""
}

func claimAsStringSlice(claims map[string]interface{}, key string) []string {
	value, exists := claims[key]
	if !exists || value == nil {
		return nil
	}
	switch v := value.(type) {
	case string:
		return []string{v}
	case []interface{}:
		result := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	default:
		return nil
	}
}

func claimAsInt64(claims map[string]interface{}, key string) int64 {
	value, exists := claims[key]
	if !exists || value == nil {
		return 0
	}
	switch v := value.(type) {
	case float64:
		return int64(v)
	case int64:
		return v
	case int:
		return int64(v)
	default:
		return 0
	}
}
