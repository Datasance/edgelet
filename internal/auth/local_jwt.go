package auth

import (
	"errors"
	"fmt"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

const (
	localJWTModuleName = "Local API JWT"
)

// LocalJWTValidationResult captures parsed/validated token metadata.
type LocalJWTValidationResult struct {
	Claims jwt.MapClaims
	Alg    string
}

// ValidateLocalJWT validates LocalAPI JWTs according to agent state:
// - unprovisioned: unsigned bootstrap JWTs are accepted
// - provisioned: unsigned JWTs are rejected and signed JWT is required
func ValidateLocalJWT(tokenString string) (*LocalJWTValidationResult, error) {
	tokenString = strings.TrimSpace(tokenString)
	if tokenString == "" {
		return nil, errors.New("empty token")
	}

	claims, alg, err := parseClaimsUnverified(tokenString)
	if err != nil {
		return nil, fmt.Errorf("failed to parse JWT: %w", err)
	}
	if err := validateRequiredTemporalClaims(claims); err != nil {
		return nil, err
	}

	provisioned, err := isProvisionedForJWT()
	if err != nil {
		return nil, fmt.Errorf("failed to resolve provisioning state: %w", err)
	}
	if provisioned {
		if strings.EqualFold(alg, "none") {
			return nil, errors.New("unsigned JWT not allowed in provisioned mode")
		}
		jm := GetJWTManager()
		if _, err := jm.ValidateJWT(tokenString); err != nil {
			return nil, fmt.Errorf("signed JWT validation failed: %w", err)
		}
		if err := validateLocalAPITokenClaims(claims); err != nil {
			return nil, err
		}
		return &LocalJWTValidationResult{Claims: claims, Alg: alg}, nil
	}

	if !strings.EqualFold(alg, "none") {
		return nil, errors.New("only unsigned bootstrap JWT is allowed when unprovisioned")
	}
	if err := validateLocalAPITokenClaims(claims); err != nil {
		return nil, err
	}
	if tokenUse, _ := claims["tokenUse"].(string); tokenUse != tokenUseLocalAPI {
		return nil, errors.New("bootstrap mode only accepts localapi tokenUse")
	}
	return &LocalJWTValidationResult{Claims: claims, Alg: alg}, nil
}

func parseClaimsUnverified(tokenString string) (jwt.MapClaims, string, error) {
	parser := jwt.NewParser()
	claims := jwt.MapClaims{}
	token, _, err := parser.ParseUnverified(tokenString, claims)
	if err != nil {
		return nil, "", err
	}

	alg, _ := token.Header["alg"].(string)
	return claims, alg, nil
}

func validateRequiredTemporalClaims(claims jwt.MapClaims) error {
	required := []string{"iat", "nbf", "exp", "jti"}
	for _, key := range required {
		if _, exists := claims[key]; !exists {
			return fmt.Errorf("missing required claim: %s", key)
		}
	}
	return nil
}

func validateLocalAPITokenClaims(claims jwt.MapClaims) error {
	iss, _ := claims["iss"].(string)
	if strings.TrimSpace(iss) != jwtIssuer {
		return fmt.Errorf("invalid issuer")
	}
	tokenUse, _ := claims["tokenUse"].(string)
	tokenUse = strings.TrimSpace(tokenUse)
	if tokenUse != tokenUseLocalAPI && tokenUse != tokenUseServiceAccount {
		return fmt.Errorf("invalid token use")
	}
	expectedAudience := localAPIAudience
	if tokenUse == tokenUseServiceAccount {
		expectedAudience = serviceAccountAudience
	}

	if audRaw, exists := claims["aud"]; exists {
		switch v := audRaw.(type) {
		case string:
			if strings.TrimSpace(v) != expectedAudience {
				return fmt.Errorf("invalid audience")
			}
		case []interface{}:
			for _, item := range v {
				if aud, ok := item.(string); ok && strings.TrimSpace(aud) == expectedAudience {
					return nil
				}
			}
			return fmt.Errorf("invalid audience")
		case []string:
			for _, aud := range v {
				if strings.TrimSpace(aud) == expectedAudience {
					return nil
				}
			}
			return fmt.Errorf("invalid audience")
		default:
			return fmt.Errorf("invalid audience")
		}
		return nil
	}
	return fmt.Errorf("missing audience")
}

func isProvisionedForJWT() (bool, error) {
	return hydrateProvisionedPrivateKeyFromDB()
}
