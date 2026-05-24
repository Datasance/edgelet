package auth

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"github.com/datasance/edgelet/internal/config"
	"github.com/datasance/edgelet/internal/store"
	"github.com/golang-jwt/jwt/v5"
)

func bootstrapClaims() jwt.MapClaims {
	now := time.Now().Unix()
	return jwt.MapClaims{
		"iat":      now,
		"nbf":      now,
		"exp":      now + 300,
		"jti":      "bootstrap-jti",
		"iss":      jwtIssuer,
		"aud":      []string{localAPIAudience},
		"tokenUse": tokenUseLocalAPI,
	}
}

func createEd25519JWKBase64(t *testing.T) string {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate key pair: %v", err)
	}
	seed := privateKey.Seed()
	jwk := map[string]interface{}{
		"kty": "OKP",
		"crv": "Ed25519",
		"d":   base64.RawURLEncoding.EncodeToString(seed),
		"x":   base64.RawURLEncoding.EncodeToString(publicKey),
	}
	jwkJSON, err := json.Marshal(jwk)
	if err != nil {
		t.Fatalf("failed to marshal JWK: %v", err)
	}
	return base64.StdEncoding.EncodeToString(jwkJSON)
}

func TestValidateLocalJWT_UnprovisionedAllowsUnsignedBootstrapToken(t *testing.T) {
	cfg := config.GetInstance()
	cfg.IOFogUUID = ""
	cfg.PrivateKey = ""

	token := jwt.NewWithClaims(jwt.SigningMethodNone, bootstrapClaims())
	tokenString, err := token.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("failed to sign unsigned bootstrap token: %v", err)
	}

	result, err := ValidateLocalJWT(tokenString)
	if err != nil {
		t.Fatalf("expected unsigned bootstrap token to be accepted while unprovisioned, got %v", err)
	}
	if result == nil || result.Alg != "none" {
		t.Fatalf("expected alg=none validation result, got %#v", result)
	}
}

func TestValidateLocalJWT_UnprovisionedRejectsSignedToken(t *testing.T) {
	cfg := config.GetInstance()
	cfg.IOFogUUID = ""
	cfg.PrivateKey = ""

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, bootstrapClaims())
	tokenString, err := token.SignedString([]byte("secret"))
	if err != nil {
		t.Fatalf("failed to sign hs256 token: %v", err)
	}

	if _, err := ValidateLocalJWT(tokenString); err == nil {
		t.Fatal("expected signed token rejection while unprovisioned")
	}
}

func TestValidateLocalJWT_ProvisionedRejectsUnsignedToken(t *testing.T) {
	openTestSQLite(t)

	cfg := config.GetInstance()
	cfg.IOFogUUID = "agent-uuid"
	cfg.PrivateKey = ""
	if err := store.GetInstance().UpsertAgentPrivateKey(createEd25519JWKBase64(t)); err != nil {
		t.Fatalf("failed to seed private key: %v", err)
	}

	token := jwt.NewWithClaims(jwt.SigningMethodNone, bootstrapClaims())
	tokenString, err := token.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("failed to sign unsigned token: %v", err)
	}

	if _, err := ValidateLocalJWT(tokenString); err == nil {
		t.Fatal("expected unsigned token rejection while provisioned")
	}
}

func TestValidateLocalJWT_ProvisionedAcceptsSignedEd25519Token(t *testing.T) {
	openTestSQLite(t)

	cfg := config.GetInstance()
	cfg.IOFogUUID = "agent-uuid"
	cfg.PrivateKey = ""
	if err := store.GetInstance().UpsertAgentPrivateKey(createEd25519JWKBase64(t)); err != nil {
		t.Fatalf("failed to seed private key: %v", err)
	}

	manager := GetJWTManager()
	manager.Reset()

	tokenString, _, _, _, err := manager.GenerateLocalAPITokenJWT("", 10*time.Minute, nil)
	if err != nil {
		t.Fatalf("failed to generate signed token: %v", err)
	}

	if _, err := ValidateLocalJWT(tokenString); err != nil {
		t.Fatalf("expected signed token to be accepted while provisioned, got %v", err)
	}
}

func TestValidateLocalJWT_ProvisionedRejectsInvalidIssuer(t *testing.T) {
	openTestSQLite(t)

	cfg := config.GetInstance()
	cfg.IOFogUUID = "agent-uuid"
	cfg.PrivateKey = ""
	if err := store.GetInstance().UpsertAgentPrivateKey(createEd25519JWKBase64(t)); err != nil {
		t.Fatalf("failed to seed private key: %v", err)
	}

	manager := GetJWTManager()
	manager.Reset()
	tokenString, _, _, _, err := manager.GenerateLocalAPITokenJWT("", 10*time.Minute, nil)
	if err != nil {
		t.Fatalf("failed to generate signed token: %v", err)
	}

	claims, alg, err := parseClaimsUnverified(tokenString)
	if err != nil {
		t.Fatalf("failed to parse token: %v", err)
	}
	claims["iss"] = "https://invalid.example"
	tampered := jwt.NewWithClaims(jwt.GetSigningMethod(alg), claims)
	priv := manager.privateKey
	tokenString, err = tampered.SignedString(priv)
	if err != nil {
		t.Fatalf("failed to sign tampered token: %v", err)
	}

	if _, err := ValidateLocalJWT(tokenString); err == nil {
		t.Fatal("expected invalid issuer rejection")
	}
}
