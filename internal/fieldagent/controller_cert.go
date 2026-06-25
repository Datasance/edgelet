package fieldagent

import (
	"crypto/tls"
	"crypto/x509"

	"github.com/eclipse-iofog/edgelet/internal/auth"
	"github.com/eclipse-iofog/edgelet/internal/utils/logging"
)

// loadControllerCert loads the controller trust certificate from config, logging
// non-fatal load failures and returning nil when unset or unreadable.
func loadControllerCert(configValue, moduleName string) *x509.Certificate {
	if configValue == "" {
		return nil
	}
	cert, err := auth.LoadControllerCertFromConfig(configValue)
	if err != nil {
		logging.LogError(moduleName, "Failed to load controller certificate", err)
		return nil
	}
	return cert
}

// controllerDialTLSConfig builds TLS settings for controller HTTPS/WSS dials.
func controllerDialTLSConfig(secureMode bool, cert *x509.Certificate) *tls.Config {
	cfg := &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: !secureMode, // #nosec G402 -- controlled by SecureMode config; false in production
	}
	if cert != nil {
		pool := x509.NewCertPool()
		pool.AddCert(cert)
		cfg.RootCAs = pool
	}
	return cfg
}
