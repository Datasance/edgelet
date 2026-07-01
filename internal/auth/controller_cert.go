package auth

import (
	"crypto/x509"
	"encoding/base64"
	"errors"
	"strings"
)

// LoadControllerCertsFromConfig loads all controller trust anchors from a config
// value that may be a file path, base64-encoded PEM bundle, or inline PEM.
func LoadControllerCertsFromConfig(value string) ([]*x509.Certificate, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}

	if strings.HasPrefix(value, "/") || strings.HasPrefix(value, "./") {
		return LoadCertificatesFromFile(value)
	}

	pemData, err := base64.StdEncoding.DecodeString(value)
	if err == nil {
		return LoadCertificatesFromPEM(pemData)
	}

	return LoadCertificatesFromPEM([]byte(value))
}

// LoadControllerCertFromConfig loads the first controller trust certificate from a
// config value. Prefer LoadControllerCertsFromConfig for TLS trust pools.
func LoadControllerCertFromConfig(value string) (*x509.Certificate, error) {
	certs, err := LoadControllerCertsFromConfig(value)
	if err != nil {
		return nil, err
	}
	if len(certs) == 0 {
		return nil, nil
	}
	return certs[0], nil
}

// CertPoolFromCertificates builds a root pool from the given certificates.
func CertPoolFromCertificates(certs []*x509.Certificate) *x509.CertPool {
	if len(certs) == 0 {
		return nil
	}
	pool := x509.NewCertPool()
	for _, cert := range certs {
		if cert != nil {
			pool.AddCert(cert)
		}
	}
	return pool
}

// ControllerTrustLoadError indicates configured controllerCert could not be loaded.
type ControllerTrustLoadError struct {
	Path string
	Err  error
}

func (e *ControllerTrustLoadError) Error() string {
	return e.Path + ": " + e.Err.Error()
}

func (e *ControllerTrustLoadError) Unwrap() error {
	return e.Err
}

// LoadControllerTrustForTLS loads controller trust material for BuildControllerDialTLSConfig.
// When configuredPath is empty, no error is returned. When the path is set but loading
// fails, a *ControllerTrustLoadError is returned (caller may fall back to OS trust).
func LoadControllerTrustForTLS(configuredPath string) ([]*x509.Certificate, error) {
	path := strings.TrimSpace(configuredPath)
	if path == "" {
		return nil, nil
	}
	certs, err := LoadControllerCertsFromConfig(path)
	if err != nil {
		return nil, &ControllerTrustLoadError{Path: path, Err: err}
	}
	if len(certs) == 0 {
		return nil, &ControllerTrustLoadError{Path: path, Err: errors.New("no certificates found")}
	}
	return certs, nil
}
