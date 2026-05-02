package auth

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"github.com/eclipse-iofog/agent/internal/config"
	"github.com/eclipse-iofog/agent/internal/store"
	"github.com/golang-jwt/jwt/v5"
)

func openTestSQLite(t *testing.T) {
	t.Helper()
	db := store.GetInstance()
	if err := db.Open(t.TempDir()); err != nil {
		t.Fatalf("failed to open sqlite for test: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
}

func TestJWTManager_GenerateJWT(t *testing.T) {
	openTestSQLite(t)

	// Setup: Create a test Ed25519 key pair
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	// Create JWK format
	seed := privateKey.Seed()
	jwk := map[string]interface{}{
		"kty": "OKP",
		"crv": "Ed25519",
		"d":   base64.RawURLEncoding.EncodeToString(seed),
		"x":   base64.RawURLEncoding.EncodeToString(publicKey),
	}

	jwkJSON, err := json.Marshal(jwk)
	if err != nil {
		t.Fatalf("Failed to marshal JWK: %v", err)
	}

	base64JWK := base64.StdEncoding.EncodeToString(jwkJSON)

	// Set up config
	cfg := config.GetInstance()
	cfg.PrivateKey = base64JWK
	cfg.IOFogUUID = "test-uuid-123"

	// Reset JWT manager
	manager := GetJWTManager()
	manager.Reset()

	// Generate JWT
	token, err := manager.GenerateJWT()
	if err != nil {
		t.Fatalf("Failed to generate JWT: %v", err)
	}

	if token == "" {
		t.Fatal("Generated token is empty")
	}

	// Validate the token
	parsedToken, err := jwt.Parse(token, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodEd25519); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return publicKey, nil
	})

	if err != nil {
		t.Fatalf("Failed to parse JWT: %v", err)
	}

	if !parsedToken.Valid {
		t.Fatal("Parsed token is not valid")
	}

	// Check claims
	claims, ok := parsedToken.Claims.(jwt.MapClaims)
	if !ok {
		t.Fatal("Failed to extract claims")
	}

	if claims["sub"] != "test-uuid-123" {
		t.Errorf("Expected subject 'test-uuid-123', got '%v'", claims["sub"])
	}

	if claims["iss"] != jwtIssuer {
		t.Errorf("Expected issuer '%s', got '%v'", jwtIssuer, claims["iss"])
	}

	// Check expiration
	exp, ok := claims["exp"].(float64)
	if !ok {
		t.Fatal("Expiration claim is not a number")
	}

	expTime := time.Unix(int64(exp), 0)
	if expTime.Before(time.Now()) {
		t.Error("Token is already expired")
	}

	if expTime.After(time.Now().Add(jwtExpiration + time.Minute)) {
		t.Error("Token expiration is too far in the future")
	}
}

func TestJWTManager_ValidateJWT(t *testing.T) {
	openTestSQLite(t)

	// Setup: Create a test Ed25519 key pair
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	// Create JWK format
	seed := privateKey.Seed()
	jwk := map[string]interface{}{
		"kty": "OKP",
		"crv": "Ed25519",
		"d":   base64.RawURLEncoding.EncodeToString(seed),
		"x":   base64.RawURLEncoding.EncodeToString(publicKey),
	}

	jwkJSON, err := json.Marshal(jwk)
	if err != nil {
		t.Fatalf("Failed to marshal JWK: %v", err)
	}

	base64JWK := base64.StdEncoding.EncodeToString(jwkJSON)

	// Set up config
	cfg := config.GetInstance()
	cfg.PrivateKey = base64JWK
	cfg.IOFogUUID = "test-uuid-123"

	// Reset JWT manager
	manager := GetJWTManager()
	manager.Reset()

	// Generate JWT
	token, err := manager.GenerateJWT()
	if err != nil {
		t.Fatalf("Failed to generate JWT: %v", err)
	}

	// Validate JWT
	validatedToken, err := manager.ValidateJWT(token)
	if err != nil {
		t.Fatalf("Failed to validate JWT: %v", err)
	}

	if !validatedToken.Valid {
		t.Fatal("Validated token is not valid")
	}
}

func TestJWTManager_Reset(t *testing.T) {
	openTestSQLite(t)

	manager := GetJWTManager()
	manager.Reset()

	// After reset, GenerateJWT should reinitialize
	cfg := config.GetInstance()
	cfg.PrivateKey = "invalid"
	cfg.IOFogUUID = "test-uuid"

	_, err := manager.GenerateJWT()
	if err == nil {
		t.Error("Expected error with invalid key, got nil")
	}
}
