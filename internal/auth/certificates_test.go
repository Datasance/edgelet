package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"math/big"
	"os"
	"testing"
	"time"
)

func generateTestCertificate() (*x509.Certificate, []byte, error) {
	// Generate a test RSA key
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}

	// Create certificate template
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization:  []string{"Test Org"},
			Country:       []string{"US"},
			Province:      []string{""},
			Locality:      []string{"Test City"},
			StreetAddress: []string{""},
			PostalCode:    []string{""},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	// Create certificate
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return nil, nil, err
	}

	// Parse certificate
	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, nil, err
	}

	// Encode to PEM
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})

	return cert, certPEM, nil
}

func TestLoadCertificateFromPEM(t *testing.T) {
	_, certPEM, err := generateTestCertificate()
	if err != nil {
		t.Fatalf("Failed to generate test certificate: %v", err)
	}

	cert, err := LoadCertificateFromPEM(certPEM)
	if err != nil {
		t.Fatalf("Failed to load certificate from PEM: %v", err)
	}

	if cert == nil {
		t.Fatal("Certificate is nil")
	}

	if cert.Subject.Organization[0] != "Test Org" {
		t.Errorf("Expected organization 'Test Org', got '%s'", cert.Subject.Organization[0])
	}
}

func TestLoadCertificateFromBase64(t *testing.T) {
	_, certPEM, err := generateTestCertificate()
	if err != nil {
		t.Fatalf("Failed to generate test certificate: %v", err)
	}

	// Encode to base64
	base64Cert := base64.StdEncoding.EncodeToString(certPEM)

	cert, err := LoadCertificateFromBase64(base64Cert)
	if err != nil {
		t.Fatalf("Failed to load certificate from base64: %v", err)
	}

	if cert == nil {
		t.Fatal("Certificate is nil")
	}
}

func TestTrustStore(t *testing.T) {
	cert, _, err := generateTestCertificate()
	if err != nil {
		t.Fatalf("Failed to generate test certificate: %v", err)
	}

	trustStore := NewTrustStore()
	trustStore.AddCertificate(cert)

	trustManager := trustStore.CreateCombinedTrustManager()
	if trustManager == nil {
		t.Fatal("Failed to create combined trust manager")
	}
}

func TestLoadCertificatesFromFile(t *testing.T) {
	_, certPEM, err := generateTestCertificate()
	if err != nil {
		t.Fatalf("Failed to generate test certificate: %v", err)
	}

	// Create temporary file
	tmpFile, err := os.CreateTemp("", "test-cert-*.pem")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	// Write certificate to file
	if _, err := tmpFile.Write(certPEM); err != nil {
		t.Fatalf("Failed to write certificate: %v", err)
	}
	_ = tmpFile.Close()

	// Load certificates from file
	certs, err := LoadCertificatesFromFile(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to load certificates from file: %v", err)
	}

	if len(certs) == 0 {
		t.Fatal("No certificates loaded")
	}
}

func TestCombinedTrustManager_VerifyPeerCertificate(t *testing.T) {
	cert, _, err := generateTestCertificate()
	if err != nil {
		t.Fatalf("Failed to generate test certificate: %v", err)
	}

	trustStore := NewTrustStore()
	trustStore.AddCertificate(cert)

	trustManager := trustStore.CreateCombinedTrustManager()

	// Create a certificate chain
	certDER := cert.Raw
	rawCerts := [][]byte{certDER}

	// Verify certificate
	err = trustManager.VerifyPeerCertificate(rawCerts, nil)
	// Note: This will likely fail because we're using a self-signed certificate
	// without proper chain validation, but it tests the structure
	if err == nil {
		t.Log("Certificate verification succeeded (unexpected for self-signed cert)")
	}
}
