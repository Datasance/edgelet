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

func TestBuildControllerTLSConfig_SecureModeAndCertFile(t *testing.T) {
	cfg := config.GetInstance()
	origCert := cfg.ControllerCert
	origSecure := cfg.SecureMode
	t.Cleanup(func() {
		cfg.ControllerCert = origCert
		cfg.SecureMode = origSecure
	})

	certPath := filepath.Join(t.TempDir(), "cert.crt")
	if err := os.WriteFile(certPath, []byte(testFieldAgentControllerPEM(t)), 0o600); err != nil {
		t.Fatalf("write cert: %v", err)
	}

	cfg.ControllerCert = certPath
	cfg.SecureMode = true

	tlsCfg := buildControllerTLSConfig(cfg.SecureMode, cfg.ControllerCert, "test")
	if tlsCfg.InsecureSkipVerify {
		t.Fatal("expected verification enabled when secureMode is on")
	}
	if tlsCfg.RootCAs == nil {
		t.Fatal("expected RootCAs when certificate file is configured")
	}
}

func TestBuildControllerTLSConfig_MissingFileUsesSystem(t *testing.T) {
	cfg := config.GetInstance()
	origCert := cfg.ControllerCert
	origSecure := cfg.SecureMode
	t.Cleanup(func() {
		cfg.ControllerCert = origCert
		cfg.SecureMode = origSecure
	})

	cfg.ControllerCert = filepath.Join(t.TempDir(), "missing.crt")
	cfg.SecureMode = true

	tlsCfg := buildControllerTLSConfig(cfg.SecureMode, cfg.ControllerCert, "test")
	if tlsCfg.InsecureSkipVerify {
		t.Fatal("expected verification enabled")
	}
	if tlsCfg.RootCAs != nil {
		t.Fatal("expected OS trust when configured cert file is missing")
	}
}

func TestLogHandler_LoadsControllerTLSFromFilePath(t *testing.T) {
	pemData := testFieldAgentControllerPEM(t)
	certPath := filepath.Join(t.TempDir(), "cert.crt")
	if err := os.WriteFile(certPath, []byte(pemData), 0o600); err != nil {
		t.Fatalf("write cert: %v", err)
	}

	cfg := config.GetInstance()
	origURL := cfg.ControllerURL
	origCert := cfg.ControllerCert
	origSecure := cfg.SecureMode
	t.Cleanup(func() {
		cfg.ControllerURL = origURL
		cfg.ControllerCert = origCert
		cfg.SecureMode = origSecure
	})

	cfg.ControllerURL = "https://192.168.1.1:51121/api/v3"
	cfg.ControllerCert = certPath
	cfg.SecureMode = true

	handler := newLogSessionWebSocketHandler("session-1", "ms-uuid", "", true)
	if handler == nil {
		t.Fatal("expected handler")
	}
	if handler.controllerTLS == nil {
		t.Fatal("expected controllerTLS loaded from file path")
	}
	if handler.controllerTLS.RootCAs == nil {
		t.Fatal("expected exclusive RootCAs from configured cert file")
	}
}

func TestExecHandler_LoadsControllerTLSFromFilePath(t *testing.T) {
	pemData := testFieldAgentControllerPEM(t)
	certPath := filepath.Join(t.TempDir(), "cert.crt")
	if err := os.WriteFile(certPath, []byte(pemData), 0o600); err != nil {
		t.Fatalf("write cert: %v", err)
	}

	cfg := config.GetInstance()
	origURL := cfg.ControllerURL
	origCert := cfg.ControllerCert
	origSecure := cfg.SecureMode
	t.Cleanup(func() {
		cfg.ControllerURL = origURL
		cfg.ControllerCert = origCert
		cfg.SecureMode = origSecure
	})

	cfg.ControllerURL = "https://192.168.1.1:51121/api/v3"
	cfg.ControllerCert = certPath
	cfg.SecureMode = true

	handler := newExecSessionWebSocketHandler("sess-1", "ms-uuid")
	if handler == nil {
		t.Fatal("expected handler")
	}
	if handler.controllerTLS == nil {
		t.Fatal("expected controllerTLS loaded from file path")
	}
	if handler.controllerTLS.RootCAs == nil {
		t.Fatal("expected exclusive RootCAs from configured cert file")
	}
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
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
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
