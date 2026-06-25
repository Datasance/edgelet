package auth

import (
	"crypto/x509"
	"errors"
	"strings"
)

// LoadControllerCertFromConfig loads a controller trust certificate from a config
// value that may be a file path, base64-encoded PEM, or inline PEM.
func LoadControllerCertFromConfig(value string) (*x509.Certificate, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}

	if strings.HasPrefix(value, "/") || strings.HasPrefix(value, "./") {
		certs, err := LoadCertificatesFromFile(value)
		if err != nil {
			return nil, err
		}
		if len(certs) == 0 {
			return nil, errors.New("no certificates found in file")
		}
		return certs[0], nil
	}

	cert, err := LoadCertificateFromBase64(value)
	if err == nil {
		return cert, nil
	}

	return LoadCertificateFromPEM([]byte(value))
}
