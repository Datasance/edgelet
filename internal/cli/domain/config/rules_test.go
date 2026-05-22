package config

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseSetArgs_DirectPairs(t *testing.T) {
	setMap, err := ParseSetArgs([]string{"networkInterface", "eth0", "-cf", "10", "secureMode", "true"})
	if err != nil {
		t.Fatalf("ParseSetArgs returned error: %v", err)
	}
	if got := setMap["networkInterface"]; got != "eth0" {
		t.Fatalf("expected networkInterface=eth0, got %v", got)
	}
	if got := setMap["changeFrequencySeconds"]; got != 10 {
		t.Fatalf("expected changeFrequencySeconds=10, got %v", got)
	}
	if got := setMap["secureMode"]; got != true {
		t.Fatalf("expected secureMode=true, got %v", got)
	}
}

func TestParseSetArgs_RejectsUnknownKey(t *testing.T) {
	_, err := ParseSetArgs([]string{"unknownKey", "value"})
	if err == nil {
		t.Fatal("expected error for unknown key")
	}
	if !strings.Contains(err.Error(), "unsupported config key") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseSetArgs_RejectsInvalidBool(t *testing.T) {
	_, err := ParseSetArgs([]string{"secureMode", "maybe"})
	if err == nil {
		t.Fatal("expected error for invalid bool")
	}
	if !strings.Contains(err.Error(), "must be one of true|false|on|off|1|0") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseSetArgs_ControllerCertRequiresReadablePEMPath(t *testing.T) {
	tempDir := t.TempDir()
	validCertPath := filepath.Join(tempDir, "controller.crt")
	if err := os.WriteFile(validCertPath, []byte(generateTestPEMCertificate(t)), 0o600); err != nil {
		t.Fatalf("failed to write test cert: %v", err)
	}

	setMap, err := ParseSetArgs([]string{"-ac", validCertPath})
	if err != nil {
		t.Fatalf("expected valid controller cert path, got error: %v", err)
	}
	if got := setMap["controllerCert"]; got != validCertPath {
		t.Fatalf("expected controllerCert path %q, got %v", validCertPath, got)
	}

	_, err = ParseSetArgs([]string{"-ac", filepath.Join(tempDir, "missing.crt")})
	if err == nil || !strings.Contains(err.Error(), "readable PEM certificate file path") {
		t.Fatalf("expected missing file validation error, got: %v", err)
	}
}

func generateTestPEMCertificate(t *testing.T) string {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "cli-test-cert",
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
