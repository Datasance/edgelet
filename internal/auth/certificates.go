package auth

import (
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/eclipse-iofog/agent-go/internal/utils/logging"
)

const (
	certModuleName = "Certificate Manager"
)

// TrustStore manages certificate trust stores
type TrustStore struct {
	mu           sync.RWMutex
	systemCerts  *x509.CertPool
	customCerts  []*x509.Certificate
}

var (
	systemCertPool *x509.CertPool
	systemPoolOnce sync.Once
)

// getSystemCertPool returns the system certificate pool (singleton)
func getSystemCertPool() *x509.CertPool {
	systemPoolOnce.Do(func() {
		var err error
		systemCertPool, err = x509.SystemCertPool()
		if err != nil {
			// Fallback to empty pool if system pool fails
			systemCertPool = x509.NewCertPool()
			logging.LogError(certModuleName, "Failed to load system cert pool, using empty pool", err)
		}
	})
	return systemCertPool
}

// NewTrustStore creates a new trust store
func NewTrustStore() *TrustStore {
	return &TrustStore{
		systemCerts: getSystemCertPool(),
		customCerts: make([]*x509.Certificate, 0),
	}
}

// AddCertificate adds a certificate to the trust store
func (ts *TrustStore) AddCertificate(cert *x509.Certificate) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.customCerts = append(ts.customCerts, cert)
}

// LoadCertificateFromPEM loads a certificate from PEM-encoded data
func LoadCertificateFromPEM(pemData []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(pemData)
	if block == nil {
		return nil, errors.New("failed to decode PEM block")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse certificate: %w", err)
	}

	return cert, nil
}

// LoadCertificateFromBase64 loads a certificate from base64-encoded PEM data
func LoadCertificateFromBase64(base64Data string) (*x509.Certificate, error) {
	pemData, err := base64.StdEncoding.DecodeString(base64Data)
	if err != nil {
		return nil, fmt.Errorf("failed to decode base64: %w", err)
	}

	return LoadCertificateFromPEM(pemData)
}

// LoadCertificatesFromFile loads certificates from a file
func LoadCertificatesFromFile(filePath string) ([]*x509.Certificate, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read certificate file: %w", err)
	}

	return LoadCertificatesFromPEM(data)
}

// LoadCertificatesFromPEM loads multiple certificates from PEM-encoded data
func LoadCertificatesFromPEM(pemData []byte) ([]*x509.Certificate, error) {
	var certs []*x509.Certificate

	for len(pemData) > 0 {
		block, rest := pem.Decode(pemData)
		if block == nil {
			break
		}

		if block.Type == "CERTIFICATE" {
			cert, err := x509.ParseCertificate(block.Bytes)
			if err != nil {
				return nil, fmt.Errorf("failed to parse certificate: %w", err)
			}
			certs = append(certs, cert)
		}

		pemData = rest
	}

	if len(certs) == 0 {
		return nil, errors.New("no certificates found in PEM data")
	}

	return certs, nil
}

// LoadCertificatesFromDirectory loads all certificates from a directory
func LoadCertificatesFromDirectory(dirPath string) ([]*x509.Certificate, error) {
	var allCerts []*x509.Certificate

	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		certs, err := LoadCertificatesFromFile(path)
		if err != nil {
			// Log but continue with other files
			logging.LogError(certModuleName, fmt.Sprintf("Failed to load certificate from %s: %v", path, err), err)
			return nil
		}

		allCerts = append(allCerts, certs...)
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to walk directory: %w", err)
	}

	return allCerts, nil
}

// CreateCombinedTrustManager creates a combined trust manager that validates against
// both system certificates and custom certificates
func (ts *TrustStore) CreateCombinedTrustManager() *CombinedTrustManager {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	return &CombinedTrustManager{
		systemCerts: ts.systemCerts,
		customCerts: ts.customCerts,
	}
}

// CombinedTrustManager implements a custom X509TrustManager that validates
// against both system and custom certificates
type CombinedTrustManager struct {
	systemCerts *x509.CertPool
	customCerts []*x509.Certificate
}

// VerifyPeerCertificate validates a certificate chain
func (ctm *CombinedTrustManager) VerifyPeerCertificate(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
	if len(rawCerts) == 0 {
		return errors.New("no certificates provided")
	}

	// Parse the certificate chain
	var certs []*x509.Certificate
	for _, rawCert := range rawCerts {
		cert, err := x509.ParseCertificate(rawCert)
		if err != nil {
			return fmt.Errorf("failed to parse certificate: %w", err)
		}
		certs = append(certs, cert)
	}

	// Try to validate against system certificates first
	if ctm.systemCerts != nil {
		opts := x509.VerifyOptions{
			Roots: ctm.systemCerts,
		}
		_, err := certs[0].Verify(opts)
		if err == nil {
			return nil
		}
	}

	// Try to validate against custom certificates
	for _, customCert := range ctm.customCerts {
		opts := x509.VerifyOptions{
			Roots: x509.NewCertPool(),
		}
		opts.Roots.AddCert(customCert)

		_, err := certs[0].Verify(opts)
		if err == nil {
			return nil
		}
	}

	return errors.New("unable to validate certificate chain against any trusted certificate")
}
