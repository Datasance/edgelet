package fieldagent

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/eclipse-iofog/edgelet/internal/config"
)

func TestControllerDialTLSConfig_SecureModeAndCert(t *testing.T) {
	cert := testFieldAgentControllerCert(t)

	secureWithCert := controllerDialTLSConfig(true, cert)
	if secureWithCert.InsecureSkipVerify {
		t.Fatal("expected verification enabled when secureMode is on")
	}
	if secureWithCert.RootCAs == nil {
		t.Fatal("expected RootCAs when certificate is configured")
	}

	insecureNoCert := controllerDialTLSConfig(false, nil)
	if !insecureNoCert.InsecureSkipVerify {
		t.Fatal("expected InsecureSkipVerify when secureMode is off")
	}
}

func TestLogHandler_LoadsControllerCertFromFilePath(t *testing.T) {
	pemData := testFieldAgentControllerPEM(t)
	certPath := filepath.Join(t.TempDir(), "cert.crt")
	if err := os.WriteFile(certPath, []byte(pemData), 0o600); err != nil {
		t.Fatalf("write cert: %v", err)
	}

	cfg := config.GetInstance()
	origURL := cfg.ControllerURL
	origCert := cfg.ControllerCert
	t.Cleanup(func() {
		cfg.ControllerURL = origURL
		cfg.ControllerCert = origCert
	})

	cfg.ControllerURL = "https://192.168.1.1:51121/api/v3"
	cfg.ControllerCert = certPath

	handler := newLogSessionWebSocketHandler("session-1", "ms-uuid", "", true)
	if handler == nil {
		t.Fatal("expected handler")
	}
	if handler.controllerCert == nil {
		t.Fatal("expected controllerCert loaded from file path")
	}
	if handler.controllerCert.Subject.CommonName != "fieldagent-wss-test" {
		t.Fatalf("unexpected cert CN: %q", handler.controllerCert.Subject.CommonName)
	}
}

func TestExecHandler_LoadsControllerCertFromFilePath(t *testing.T) {
	pemData := testFieldAgentControllerPEM(t)
	certPath := filepath.Join(t.TempDir(), "cert.crt")
	if err := os.WriteFile(certPath, []byte(pemData), 0o600); err != nil {
		t.Fatalf("write cert: %v", err)
	}

	cfg := config.GetInstance()
	origURL := cfg.ControllerURL
	origCert := cfg.ControllerCert
	t.Cleanup(func() {
		cfg.ControllerURL = origURL
		cfg.ControllerCert = origCert
	})

	cfg.ControllerURL = "https://192.168.1.1:51121/api/v3"
	cfg.ControllerCert = certPath

	handler := newExecSessionWebSocketHandler("sess-1", "ms-uuid")
	if handler == nil {
		t.Fatal("expected handler")
	}
	if handler.controllerCert == nil {
		t.Fatal("expected controllerCert loaded from file path")
	}
}

func testFieldAgentControllerCert(t *testing.T) *x509.Certificate {
	t.Helper()
	cert, err := x509.ParseCertificate(testFieldAgentControllerDER(t))
	if err != nil {
		t.Fatalf("parse cert: %v", err)
	}
	return cert
}

func testFieldAgentControllerDER(t *testing.T) []byte {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "fieldagent-wss-test",
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
	return der
}

func testFieldAgentControllerPEM(t *testing.T) string {
	t.Helper()
	return string(pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: testFieldAgentControllerDER(t),
	}))
}
