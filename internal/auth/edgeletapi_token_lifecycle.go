package auth

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/datasance/edgelet/internal/config"
	"github.com/datasance/edgelet/internal/store"
	"github.com/datasance/edgelet/internal/utils"
	"github.com/golang-jwt/jwt/v5"
)

const (
	bootstrapEdgeletAPISubject = "system:edgeletadmin:bootstrap"
	bootstrapEdgeletAPITTL     = 10 * time.Minute
)

// EnsureEdgeletAPITokenForCurrentState reconciles /etc/edgelet/edgelet-api token contents.
// - Unprovisioned: unsigned bootstrap JWT
// - Provisioned: signed Ed25519 JWT
func EnsureEdgeletAPITokenForCurrentState() error {
	tokenManager := GetLocalTokenManager()
	if err := os.MkdirAll(utils.GetConfigDir(), 0700); err != nil {
		return fmt.Errorf("failed to ensure config directory for edgelet-api JWT: %w", err)
	}

	isProvisioned, err := hydrateProvisionedPrivateKeyFromDB()
	if err != nil {
		return fmt.Errorf("failed to resolve provisioning state from sqlite: %w", err)
	}
	if !isProvisioned {
		token, err := GenerateBootstrapEdgeletAPIJWT(bootstrapEdgeletAPITTL)
		if err != nil {
			return fmt.Errorf("failed to generate bootstrap edgelet-api JWT: %w", err)
		}
		if err := tokenManager.SaveToken(token); err != nil {
			return fmt.Errorf("failed to save bootstrap edgelet-api JWT: %w", err)
		}
		return nil
	}

	extraClaims := map[string]any{
		"edgelet.iofog.org": map[string]any{
			"rules": map[string]any{
				"*": []any{"*"},
			},
		},
	}
	token, _, _, _, err := GetJWTManager().GenerateEdgeletAPITokenJWT("", bootstrapEdgeletAPITTL, extraClaims)
	if err != nil {
		return fmt.Errorf("failed to generate signed edgelet-api JWT: %w", err)
	}
	if err := tokenManager.SaveToken(token); err != nil {
		return fmt.Errorf("failed to save signed edgelet-api JWT: %w", err)
	}
	return nil
}

// GenerateBootstrapEdgeletAPIJWT creates an unsigned JWT for unprovisioned bootstrap mode.
func GenerateBootstrapEdgeletAPIJWT(ttl time.Duration) (string, error) {
	if ttl <= 0 {
		ttl = bootstrapEdgeletAPITTL
	}
	now := time.Now()
	claims := jwt.MapClaims{
		"sub":      bootstrapEdgeletAPISubject,
		"iss":      jwtIssuer,
		"aud":      []string{edgeletAPIAudience},
		"exp":      now.Add(ttl).Unix(),
		"iat":      now.Unix(),
		"nbf":      now.Unix(),
		"tokenUse": tokenUseEdgeletAPI,
		"edgelet.iofog.org": map[string]any{
			"rules": map[string]any{
				"*": []any{"*"},
			},
		},
	}
	jti, err := generateJWTID(claims)
	if err != nil {
		return "", err
	}
	claims["jti"] = jti
	token := jwt.NewWithClaims(jwt.SigningMethodNone, claims)
	return token.SignedString(jwt.UnsafeAllowNoneSignatureType)
}

func hydrateProvisionedPrivateKeyFromDB() (bool, error) {
	cfg := config.GetInstance()
	if strings.TrimSpace(cfg.IOFogUUID) == "" {
		return false, nil
	}

	db := store.GetInstance()
	privateKey, found, err := db.GetAgentPrivateKey()
	if err != nil {
		return false, err
	}
	if !found || strings.TrimSpace(privateKey) == "" {
		cfg.PrivateKey = ""
		return false, nil
	}

	// JWT manager reads from config, so keep it hydrated from DB.
	cfg.PrivateKey = privateKey
	return true, nil
}

// ShouldRotateEdgeletAPIToken returns true when token should be rotated
// according to token lifetime policy.
func ShouldRotateEdgeletAPIToken(tokenString string, now time.Time) (bool, error) {
	claims, _, err := parseClaimsUnverified(strings.TrimSpace(tokenString))
	if err != nil {
		return true, err
	}
	iat, err := claimAsInt64(claims, "iat")
	if err != nil {
		return true, err
	}
	exp, err := claimAsInt64(claims, "exp")
	if err != nil {
		return true, err
	}
	return ShouldRotateByLifetime(iat, exp, now), nil
}

func claimAsInt64(claims jwt.MapClaims, key string) (int64, error) {
	raw, ok := claims[key]
	if !ok {
		return 0, fmt.Errorf("missing %s claim", key)
	}
	switch v := raw.(type) {
	case float64:
		return int64(v), nil
	case int64:
		return v, nil
	case int:
		return int64(v), nil
	case json.Number:
		return v.Int64()
	case string:
		return strconv.ParseInt(v, 10, 64)
	default:
		return 0, fmt.Errorf("invalid %s claim type", key)
	}
}
