package auth

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadControllerCertFromConfig_FilePath(t *testing.T) {
	pemData := testControllerPEM(t)
	path := filepath.Join(t.TempDir(), "controller.crt")
	if err := os.WriteFile(path, []byte(pemData), 0o600); err != nil {
		t.Fatalf("write cert file: %v", err)
	}

	cert, err := LoadControllerCertFromConfig(path)
	if err != nil {
		t.Fatalf("LoadControllerCertFromConfig(file): %v", err)
	}
	if cert == nil {
		t.Fatal("expected certificate")
	}
	if cert.Subject.CommonName != "controller-test-cert" {
		t.Fatalf("CN = %q, want controller-test-cert", cert.Subject.CommonName)
	}
}

func TestLoadControllerCertFromConfig_Base64(t *testing.T) {
	pemData := testControllerPEM(t)
	encoded := base64.StdEncoding.EncodeToString([]byte(pemData))

	cert, err := LoadControllerCertFromConfig(encoded)
	if err != nil {
		t.Fatalf("LoadControllerCertFromConfig(base64): %v", err)
	}
	if cert == nil {
		t.Fatal("expected certificate")
	}
}

func TestLoadControllerCertFromConfig_InlinePEM(t *testing.T) {
	pemData := testControllerPEM(t)

	cert, err := LoadControllerCertFromConfig(pemData)
	if err != nil {
		t.Fatalf("LoadControllerCertFromConfig(pem): %v", err)
	}
	if cert == nil {
		t.Fatal("expected certificate")
	}
}

func TestLoadControllerCertFromConfig_Empty(t *testing.T) {
	cert, err := LoadControllerCertFromConfig("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cert != nil {
		t.Fatal("expected nil certificate for empty value")
	}
}

func TestLoadControllerCertFromConfig_InvalidPath(t *testing.T) {
	_, err := LoadControllerCertFromConfig("/etc/edgelet/does-not-exist.crt")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func testControllerPEM(t *testing.T) string {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "controller-test-cert",
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
