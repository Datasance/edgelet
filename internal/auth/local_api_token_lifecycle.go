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
	bootstrapLocalAPISubject = "system:localadmin:bootstrap"
	bootstrapLocalAPITTL     = 10 * time.Minute
)

// EnsureLocalAPITokenForCurrentState reconciles /etc/edgelet/local-api token contents.
// - Unprovisioned: unsigned bootstrap JWT
// - Provisioned: signed Ed25519 JWT
func EnsureLocalAPITokenForCurrentState() error {
	tokenManager := GetLocalTokenManager()
	if err := os.MkdirAll(utils.GetConfigDir(), 0700); err != nil {
		return fmt.Errorf("failed to ensure config directory for local-api JWT: %w", err)
	}

	isProvisioned, err := hydrateProvisionedPrivateKeyFromDB()
	if err != nil {
		return fmt.Errorf("failed to resolve provisioning state from sqlite: %w", err)
	}
	if !isProvisioned {
		token, err := GenerateBootstrapLocalAPIJWT(bootstrapLocalAPITTL)
		if err != nil {
			return fmt.Errorf("failed to generate bootstrap local-api JWT: %w", err)
		}
		if err := tokenManager.SaveToken(token); err != nil {
			return fmt.Errorf("failed to save bootstrap local-api JWT: %w", err)
		}
		return nil
	}

	extraClaims := map[string]interface{}{
		"edgelet.iofog.org": map[string]interface{}{
			"rules": map[string]interface{}{
				"*": []interface{}{"*"},
			},
		},
	}
	token, _, _, _, err := GetJWTManager().GenerateLocalAPITokenJWT("", bootstrapLocalAPITTL, extraClaims)
	if err != nil {
		return fmt.Errorf("failed to generate signed local-api JWT: %w", err)
	}
	if err := tokenManager.SaveToken(token); err != nil {
		return fmt.Errorf("failed to save signed local-api JWT: %w", err)
	}
	return nil
}

// GenerateBootstrapLocalAPIJWT creates an unsigned JWT for unprovisioned bootstrap mode.
func GenerateBootstrapLocalAPIJWT(ttl time.Duration) (string, error) {
	if ttl <= 0 {
		ttl = bootstrapLocalAPITTL
	}
	now := time.Now()
	claims := jwt.MapClaims{
		"sub":      bootstrapLocalAPISubject,
		"iss":      jwtIssuer,
		"aud":      []string{localAPIAudience},
		"exp":      now.Add(ttl).Unix(),
		"iat":      now.Unix(),
		"nbf":      now.Unix(),
		"tokenUse": tokenUseLocalAPI,
		"edgelet.iofog.org": map[string]interface{}{
			"rules": map[string]interface{}{
				"*": []interface{}{"*"},
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

// ShouldRotateLocalAPIToken returns true when token should be rotated
// according to token lifetime policy.
func ShouldRotateLocalAPIToken(tokenString string, now time.Time) (bool, error) {
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
