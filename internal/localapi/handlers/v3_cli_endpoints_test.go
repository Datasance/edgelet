package handlers

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/eclipse-iofog/agent/internal/config"
	"github.com/eclipse-iofog/agent/internal/models"
	"github.com/eclipse-iofog/agent/internal/store"
	"github.com/eclipse-iofog/agent/internal/utils"
)

func TestHandleSystemControllerCert_DecodesBase64WritesPathEnablesSecureMode(t *testing.T) {
	cfg := setupConfigForGPSTests(t)
	certPath := filepath.Join(t.TempDir(), "controller-ca.crt")
	errorsMap := cfg.SetConfig(map[string]interface{}{"ac": certPath})
	if len(errorsMap) > 0 {
		t.Fatalf("failed to set certificate path: %v", errorsMap)
	}
	reloadCalled := false
	cfg.SetReloadCallback(func() error {
		reloadCalled = true
		return nil
	})

	handler := NewV3Handler()
	pemCert := generateTestCertPEM(t)
	base64Cert := base64.StdEncoding.EncodeToString([]byte(pemCert))
	reqBody := []byte(`{"certificate":` + strconv.Quote(base64Cert) + `}`)
	req := httptest.NewRequest(http.MethodPost, "/v3/system/controller/cert", bytes.NewBuffer(reqBody))
	rec := httptest.NewRecorder()

	handler.HandleSystemControllerCert(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !reloadCalled {
		t.Fatal("expected reload callback to be called")
	}
	fileData, err := os.ReadFile(certPath)
	if err != nil {
		t.Fatalf("expected certificate file to be written: %v", err)
	}
	if strings.TrimSpace(string(fileData)) != strings.TrimSpace(pemCert) {
		t.Fatalf("certificate file does not match decoded PEM content")
	}
	if !cfg.SecureMode {
		t.Fatal("expected secure mode to be enabled")
	}
}

func TestHandleSystemControllerCert_RejectsNonBase64Input(t *testing.T) {
	cfg := setupConfigForGPSTests(t)
	certPath := filepath.Join(t.TempDir(), "controller-ca.crt")
	errorsMap := cfg.SetConfig(map[string]interface{}{"ac": certPath})
	if len(errorsMap) > 0 {
		t.Fatalf("failed to set certificate path: %v", errorsMap)
	}
	handler := NewV3Handler()
	req := httptest.NewRequest(http.MethodPost, "/v3/system/controller/cert", bytes.NewBufferString(`{"certificate":"-----BEGIN CERTIFICATE-----bad"}`))
	rec := httptest.NewRecorder()

	handler.HandleSystemControllerCert(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandleSystemConfigSwitch_SwitchesProfileAndTriggersReload(t *testing.T) {
	cfg := setupConfigForGPSTests(t)
	cfg.SetCurrentProfile(utils.ConfigSwitcherStateDefault)
	reloadCalled := false
	cfg.SetReloadCallback(func() error {
		reloadCalled = true
		return nil
	})

	handler := NewV3Handler()
	req := httptest.NewRequest(http.MethodPost, "/v3/system/config/switch", bytes.NewBufferString(`{"profile":"dev"}`))
	rec := httptest.NewRecorder()
	handler.HandleSystemConfigSwitch(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if got := config.GetInstance().GetCurrentProfile().FullValue(); got != "development" {
		t.Fatalf("expected switched profile development, got %q", got)
	}
	if !reloadCalled {
		t.Fatal("expected reload callback to be called")
	}
}

func TestHandleConfig_RejectsInvalidNetworkInterfaceWithoutPersisting(t *testing.T) {
	cfg := setupConfigForGPSTests(t)
	cfg.ControllerURL = "http://127.0.0.1:51121"
	cfg.NetworkInterface = "dynamic"

	handler := NewV3Handler()
	req := httptest.NewRequest(http.MethodPatch, "/v3/system/config", bytes.NewBufferString(`{"set":{"networkInterface":"iface-does-not-exist-98765"}}`))
	rec := httptest.NewRecorder()
	handler.HandleConfig(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	var envelope struct {
		Success bool `json:"success"`
		Error   struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if envelope.Success {
		t.Fatalf("expected success=false for invalid network interface, body=%s", rec.Body.String())
	}
	if envelope.Error.Code != ErrCodeInvalidArgument {
		t.Fatalf("expected error code %s, got %s", ErrCodeInvalidArgument, envelope.Error.Code)
	}
	if cfg.NetworkInterface != "dynamic" {
		t.Fatalf("expected networkInterface to remain unchanged, got %q", cfg.NetworkInterface)
	}
}

func TestHandleSystemProvisionDelete_RejectsInvalidScope(t *testing.T) {
	handler := NewV3Handler()
	req := httptest.NewRequest(http.MethodDelete, "/v3/system/provision?scope=bad", nil)
	rec := httptest.NewRecorder()

	handler.HandleSystemProvision(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandleSystemPrune_RejectsInvalidMode(t *testing.T) {
	handler := NewV3Handler()
	req := httptest.NewRequest(http.MethodPost, "/v3/system/prune?mode=bad", nil)
	rec := httptest.NewRecorder()

	handler.HandleSystemPrune(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandleSystemLogs_RejectsInvalidTail(t *testing.T) {
	cfg := setupConfigForGPSTests(t)
	cfg.LogDiskDirectory = t.TempDir() + string(os.PathSeparator)

	handler := NewV3Handler()
	req := httptest.NewRequest(http.MethodGet, "/v3/system/logs?tailLines=bad", nil)
	rec := httptest.NewRecorder()

	handler.HandleSystemLogs(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandleSystemLogs_BoundedReturnsEntries(t *testing.T) {
	cfg := setupConfigForGPSTests(t)
	cfg.LogDiskDirectory = t.TempDir() + string(os.PathSeparator)
	logFile := filepath.Join(cfg.LogDiskDirectory, "iofog-agent.0.log")
	logLines := strings.Join([]string{
		"2026-05-17 00:00:01.000 [info] boot",
		"2026-05-17 00:00:02.000 [info] ready",
	}, "\n") + "\n"
	if err := os.WriteFile(logFile, []byte(logLines), 0o600); err != nil {
		t.Fatalf("failed to write log file: %v", err)
	}

	handler := NewV3Handler()
	req := httptest.NewRequest(http.MethodGet, "/v3/system/logs?tailLines=2", nil)
	rec := httptest.NewRecorder()

	handler.HandleSystemLogs(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var envelope struct {
		Success bool `json:"success"`
		Data    struct {
			Entries []map[string]interface{} `json:"entries"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("failed to parse response: %v body=%s", err, rec.Body.String())
	}
	if !envelope.Success {
		t.Fatalf("expected success=true body=%s", rec.Body.String())
	}
	if len(envelope.Data.Entries) < 2 {
		t.Fatalf("expected at least 2 log entries, got %d body=%s", len(envelope.Data.Entries), rec.Body.String())
	}
}

func TestHandleDeployRegistries_GetByID_IncludesPassword(t *testing.T) {
	db := store.GetInstance()
	_ = db.Close()
	if err := db.Open(t.TempDir()); err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	if err := db.EnsureDefaultLocalRegistries(); err != nil {
		t.Fatalf("failed to seed default registries: %v", err)
	}
	if err := db.UpsertLocalRegistry(models.NewRegistry(7, "private.example.com", false, "john", "s3cr3t", "john@example.com")); err != nil {
		t.Fatalf("failed to upsert local registry: %v", err)
	}

	handler := NewV3Handler()
	req := httptest.NewRequest(http.MethodGet, "/v3/deploy/registries/7", nil)
	rec := httptest.NewRecorder()

	handler.HandleDeployRegistries(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var envelope struct {
		Success bool `json:"success"`
		Data    struct {
			ID       int    `json:"id"`
			Password string `json:"password"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("failed to parse response: %v body=%s", err, rec.Body.String())
	}
	if !envelope.Success {
		t.Fatalf("expected success=true body=%s", rec.Body.String())
	}
	if envelope.Data.ID != 7 {
		t.Fatalf("expected id=7, got %d", envelope.Data.ID)
	}
	if envelope.Data.Password != "s3cr3t" {
		t.Fatalf("expected password in data payload, got %q", envelope.Data.Password)
	}
}

func generateTestCertPEM(t *testing.T) string {
	t.Helper()

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "test-cert",
		},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, pub, priv)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}
