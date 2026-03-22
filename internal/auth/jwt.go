package auth

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/eclipse-iofog/agent/internal/config"
	"github.com/eclipse-iofog/agent/internal/utils/logging"
	"github.com/golang-jwt/jwt/v5"
)

const (
	moduleName    = "JWT Manager"
	jwtExpiration = 10 * time.Minute
	jwtIssuer     = "iofog-agent"
)

// JWTManager handles JWT token generation and validation
type JWTManager struct {
	mu          sync.RWMutex
	privateKey  ed25519.PrivateKey
	keyID       string
	initialized bool
}

var (
	instance *JWTManager
	once     sync.Once
)

// GetInstance returns the singleton JWT manager instance
func GetJWTManager() *JWTManager {
	once.Do(func() {
		instance = &JWTManager{}
	})
	return instance
}

// Reset resets the JWT Manager state to allow re-initialization with new credentials
func (j *JWTManager) Reset() {
	j.mu.Lock()
	defer j.mu.Unlock()

	logging.LogDebug(moduleName, "Resetting JWT Manager static state")
	j.privateKey = nil
	j.keyID = ""
	j.initialized = false
	logging.LogDebug(moduleName, "JWT Manager static state reset completed")
}

// loadPrivateKeyFromConfig loads and initializes the private key from config
func (j *JWTManager) loadPrivateKeyFromConfig() error {
	cfg := config.GetInstance()
	base64Key := cfg.PrivateKey

	if base64Key == "" {
		return errors.New("private key is not configured")
	}

	// Decode base64-encoded JWK
	keyBytes, err := base64.StdEncoding.DecodeString(base64Key)
	if err != nil {
		return fmt.Errorf("failed to decode base64 key: %w", err)
	}

	// Parse JWK JSON
	var jwk JWK
	if err := json.Unmarshal(keyBytes, &jwk); err != nil {
		return fmt.Errorf("failed to parse JWK JSON: %w", err)
	}

	// Validate key type and curve
	if jwk.Kty != "OKP" {
		return errors.New("key must be OKP type")
	}
	if jwk.Crv != "Ed25519" {
		return errors.New("key must use Ed25519 curve")
	}

	// Decode the private key (d parameter in JWK)
	privateKeyBytes, err := base64.RawURLEncoding.DecodeString(jwk.D)
	if err != nil {
		return fmt.Errorf("failed to decode private key: %w", err)
	}

	// Ed25519 private key is 32 bytes
	if len(privateKeyBytes) != ed25519.SeedSize {
		return fmt.Errorf("invalid private key length: expected %d, got %d", ed25519.SeedSize, len(privateKeyBytes))
	}

	// Generate key ID if not provided
	keyID := jwk.Kid
	if keyID == "" {
		keyID = generateKeyID()
		logging.LogDebug(moduleName, fmt.Sprintf("Generated key ID: %s", keyID))
	}

	// Create Ed25519 private key from seed
	privateKey := ed25519.NewKeyFromSeed(privateKeyBytes)

	j.privateKey = privateKey
	j.keyID = keyID
	j.initialized = true

	logging.LogDebug(moduleName, fmt.Sprintf("Successfully initialized Ed25519 signer with key ID: %s", keyID))
	return nil
}

// GenerateJWT generates a JWT token with the configured private key
func (j *JWTManager) GenerateJWT() (string, error) {
	j.mu.Lock()
	defer j.mu.Unlock()

	// Get UUID and private key from config first to check if agent is provisioned
	cfg := config.GetInstance()
	uuid := cfg.IOFogUUID
	privateKey := cfg.PrivateKey

	// If agent is not provisioned, return error without logging (expected state)
	if uuid == "" || privateKey == "" {
		return "", errors.New("agent is not provisioned")
	}

	// Initialize signer if not already done
	if !j.initialized {
		if err := j.loadPrivateKeyFromConfig(); err != nil {
			// Only log error if agent is provisioned (has UUID) but key loading failed
			if uuid != "" {
				logging.LogError(moduleName, fmt.Sprintf("Failed to initialize signer: %v", err), err)
			}
			return "", err
		}
	}

	// Create JWT claims
	now := time.Now()
	claims := jwt.MapClaims{
		"sub": uuid,
		"iss": jwtIssuer,
		"exp": now.Add(jwtExpiration).Unix(),
		"iat": now.Unix(),
		"jti": generateJWTID(),
		"kid": j.keyID,
	}

	// Create token with EdDSA algorithm
	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)

	// Set key ID in header
	token.Header["kid"] = j.keyID

	// Sign token with Ed25519 private key
	// The JWT library expects the full 64-byte Ed25519 private key
	tokenString, err := token.SignedString(j.privateKey)
	if err != nil {
		logging.LogError(moduleName, fmt.Sprintf("Failed to generate JWT: %v", err), err)
		return "", err
	}

	logging.LogDebug(moduleName, fmt.Sprintf("Generated JWT with key ID: %s", j.keyID))
	logging.LogDebug(moduleName, fmt.Sprintf("JTI of Generated JWT: %s", token.Claims.(jwt.MapClaims)["jti"]))
	return tokenString, nil
}

// ValidateJWT validates a JWT token using the public key derived from the private key
func (j *JWTManager) ValidateJWT(tokenString string) (*jwt.Token, error) {
	j.mu.RLock()
	defer j.mu.RUnlock()

	if !j.initialized {
		return nil, errors.New("JWT manager not initialized")
	}

	// Get public key from private key
	publicKey := j.privateKey.Public().(ed25519.PublicKey)

	// Parse and validate token
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		// Verify signing method
		if _, ok := token.Method.(*jwt.SigningMethodEd25519); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return publicKey, nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to parse JWT: %w", err)
	}

	if !token.Valid {
		return nil, errors.New("invalid JWT token")
	}

	return token, nil
}

// JWK represents a JSON Web Key structure
type JWK struct {
	Kty string `json:"kty"` // Key type (must be "OKP")
	Crv string `json:"crv"` // Curve (must be "Ed25519")
	Kid string `json:"kid"` // Key ID (optional)
	D   string `json:"d"`   // Private key (base64url encoded)
	X   string `json:"x"`   // Public key (base64url encoded, optional for private key)
}

// generateKeyID generates a unique key ID
func generateKeyID() string {
	// Simple UUID-like generation (can be improved with proper UUID library)
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

// generateJWTID generates a unique JWT ID
func generateJWTID() string {
	// Simple UUID-like generation (can be improved with proper UUID library)
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
