package auth

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/datasance/edgelet/internal/config"
	"github.com/datasance/edgelet/internal/store"
	"github.com/datasance/edgelet/internal/utils"
)

func TestEnsureLocalAPITokenForCurrentState_UnprovisionedWritesUnsignedJWT(t *testing.T) {
	originalSnapCommon := utils.SNAPCommon
	utils.SNAPCommon = t.TempDir()
	t.Cleanup(func() {
		utils.SNAPCommon = originalSnapCommon
	})

	cfg := config.GetInstance()
	cfg.IOFogUUID = ""
	cfg.PrivateKey = ""

	GetJWTManager().Reset()
	localTokenManager := GetLocalTokenManager()
	localTokenManager.Reset()

	if err := EnsureLocalAPITokenForCurrentState(); err != nil {
		t.Fatalf("failed to reconcile local-api token: %v", err)
	}

	token, err := localTokenManager.LoadToken()
	if err != nil {
		t.Fatalf("failed to load reconciled token: %v", err)
	}
	_, alg, err := parseClaimsUnverified(strings.TrimSpace(token))
	if err != nil {
		t.Fatalf("failed to parse token: %v", err)
	}
	if alg != "none" {
		t.Fatalf("expected unsigned bootstrap JWT (alg=none), got %s", alg)
	}
	claims, _, err := parseClaimsUnverified(strings.TrimSpace(token))
	if err != nil {
		t.Fatalf("failed to parse bootstrap claims: %v", err)
	}
	jti, _ := claims["jti"].(string)
	if matched, _ := regexp.MatchString(`^[A-Z2-7]+$`, jti); !matched {
		t.Fatalf("expected base32 hash-like jti, got %q", jti)
	}
	if _, exists := claims["kid"]; exists {
		t.Fatal("kid must not be present in bootstrap payload")
	}
}

func TestShouldRotateLocalAPIToken(t *testing.T) {
	now := time.Now()
	token, err := GenerateBootstrapLocalAPIJWT(10 * time.Minute)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}
	shouldRotate, err := ShouldRotateLocalAPIToken(token, now.Add(8*time.Minute))
	if err != nil {
		t.Fatalf("rotation check failed: %v", err)
	}
	if !shouldRotate {
		t.Fatal("expected token to rotate near expiry")
	}
}

func TestEnsureLocalAPITokenForCurrentState_ProvisionedWritesSignedJWT(t *testing.T) {
	openTestSQLite(t)

	originalSnapCommon := utils.SNAPCommon
	utils.SNAPCommon = t.TempDir()
	t.Cleanup(func() {
		utils.SNAPCommon = originalSnapCommon
	})

	cfg := config.GetInstance()
	cfg.IOFogUUID = "agent-uuid"
	cfg.PrivateKey = ""
	if err := store.GetInstance().UpsertAgentPrivateKey(createEd25519JWKBase64(t)); err != nil {
		t.Fatalf("failed to seed private key in sqlite: %v", err)
	}

	GetJWTManager().Reset()
	localTokenManager := GetLocalTokenManager()
	localTokenManager.Reset()

	if err := EnsureLocalAPITokenForCurrentState(); err != nil {
		t.Fatalf("failed to reconcile local-api token: %v", err)
	}

	token, err := localTokenManager.LoadToken()
	if err != nil {
		t.Fatalf("failed to load reconciled token: %v", err)
	}
	_, alg, err := parseClaimsUnverified(strings.TrimSpace(token))
	if err != nil {
		t.Fatalf("failed to parse token: %v", err)
	}
	if strings.EqualFold(alg, "none") {
		t.Fatalf("expected signed JWT in provisioned mode, got alg=%s", alg)
	}
	if _, err := ValidateLocalJWT(strings.TrimSpace(token)); err != nil {
		t.Fatalf("expected provisioned signed JWT to validate, got error: %v", err)
	}
}

func TestEnsureLocalAPITokenForCurrentState_ProvisionedWithEmptyConfigPrivateKeyStillSigns(t *testing.T) {
	openTestSQLite(t)

	originalSnapCommon := utils.SNAPCommon
	utils.SNAPCommon = t.TempDir()
	t.Cleanup(func() {
		utils.SNAPCommon = originalSnapCommon
	})

	cfg := config.GetInstance()
	cfg.IOFogUUID = "agent-uuid"
	cfg.PrivateKey = ""
	if err := store.GetInstance().UpsertAgentPrivateKey(createEd25519JWKBase64(t)); err != nil {
		t.Fatalf("failed to seed private key in sqlite: %v", err)
	}

	GetJWTManager().Reset()
	localTokenManager := GetLocalTokenManager()
	localTokenManager.Reset()

	if err := EnsureLocalAPITokenForCurrentState(); err != nil {
		t.Fatalf("failed to reconcile local-api token: %v", err)
	}

	token, err := localTokenManager.LoadToken()
	if err != nil {
		t.Fatalf("failed to load reconciled token: %v", err)
	}
	_, alg, err := parseClaimsUnverified(strings.TrimSpace(token))
	if err != nil {
		t.Fatalf("failed to parse token: %v", err)
	}
	if strings.EqualFold(alg, "none") {
		t.Fatalf("expected signed JWT in provisioned mode, got alg=%s", alg)
	}
}

func TestEnsureLocalAPITokenForCurrentState_CreatesConfigDirectory(t *testing.T) {
	originalSnapCommon := utils.SNAPCommon
	snapRoot := filepath.Join(t.TempDir(), "snap-common")
	utils.SNAPCommon = snapRoot
	t.Cleanup(func() {
		utils.SNAPCommon = originalSnapCommon
	})

	cfgDir := utils.GetConfigDir()
	_ = os.RemoveAll(cfgDir)

	cfg := config.GetInstance()
	cfg.IOFogUUID = ""
	cfg.PrivateKey = ""

	GetJWTManager().Reset()
	localTokenManager := GetLocalTokenManager()
	localTokenManager.Reset()

	if err := EnsureLocalAPITokenForCurrentState(); err != nil {
		t.Fatalf("failed to reconcile local-api token with missing config dir: %v", err)
	}

	if _, err := os.Stat(cfgDir); err != nil {
		t.Fatalf("expected config dir to be created at %s, stat error: %v", cfgDir, err)
	}
}
