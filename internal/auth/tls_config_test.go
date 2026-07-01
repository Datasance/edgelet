package auth

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
)

func TestBuildControllerDialTLSConfig_SecureModeOff(t *testing.T) {
	cfg := BuildControllerDialTLSConfig(false, "/etc/edgelet/cert.crt", []*x509.Certificate{{}}, os.ErrNotExist)
	if !cfg.InsecureSkipVerify {
		t.Fatal("expected InsecureSkipVerify when secureMode is off")
	}
}

func TestBuildControllerDialTLSConfig_SecureEmptyPathUsesSystem(t *testing.T) {
	cfg := BuildControllerDialTLSConfig(true, "", nil, nil)
	if cfg.InsecureSkipVerify {
		t.Fatal("expected verification enabled")
	}
	if cfg.RootCAs != nil {
		t.Fatal("expected nil RootCAs for OS trust")
	}
}

func TestBuildControllerDialTLSConfig_SecureLoadErrorUsesSystem(t *testing.T) {
	cfg := BuildControllerDialTLSConfig(true, "/etc/edgelet/cert.crt", nil, os.ErrNotExist)
	if cfg.InsecureSkipVerify {
		t.Fatal("expected verification enabled")
	}
	if cfg.RootCAs != nil {
		t.Fatal("expected nil RootCAs when configured file is missing")
	}
}

func TestBuildControllerDialTLSConfig_SecureExclusiveCustomPool(t *testing.T) {
	cert := testControllerDialCert(t, "controller-test-cert")
	cfg := BuildControllerDialTLSConfig(true, "/etc/edgelet/cert.crt", []*x509.Certificate{cert}, nil)
	if cfg.InsecureSkipVerify {
		t.Fatal("expected verification enabled")
	}
	if cfg.RootCAs == nil {
		t.Fatal("expected exclusive RootCAs pool")
	}
}

func TestLoadControllerCertsFromConfig_MultiPEMFile(t *testing.T) {
	first := testControllerDialCert(t, "controller-first-cert")
	second := testControllerDialCert(t, "controller-second-cert")

	path := filepath.Join(t.TempDir(), "bundle.crt")
	data := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: first.Raw})
	data = append(data, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: second.Raw})...)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write bundle: %v", err)
	}

	certs, err := LoadControllerCertsFromConfig(path)
	if err != nil {
		t.Fatalf("LoadControllerCertsFromConfig: %v", err)
	}
	if len(certs) != 2 {
		t.Fatalf("expected 2 certificates, got %d", len(certs))
	}
	if certs[0].Subject.CommonName != "controller-first-cert" {
		t.Fatalf("first CN = %q", certs[0].Subject.CommonName)
	}
	if certs[1].Subject.CommonName != "controller-second-cert" {
		t.Fatalf("second CN = %q", certs[1].Subject.CommonName)
	}

	pool := CertPoolFromCertificates(certs)
	if pool == nil {
		t.Fatal("expected cert pool from bundle")
	}
}

func testControllerDialCert(t *testing.T, commonName string) *x509.Certificate {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: commonName,
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
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse cert: %v", err)
	}
	return cert
}
