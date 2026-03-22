package auth

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
)

// CreateControllerTLSConfig creates a TLS config for controller connections
func CreateControllerTLSConfig(controllerCert *x509.Certificate) (*tls.Config, error) {
	trustStore := NewTrustStore()
	if controllerCert != nil {
		trustStore.AddCertificate(controllerCert)
	}

	trustManager := trustStore.CreateCombinedTrustManager()

	return &tls.Config{
		RootCAs:            trustStore.systemCerts,
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: false,
		VerifyPeerCertificate: func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
			if err := trustManager.VerifyPeerCertificate(rawCerts, verifiedChains); err != nil {
				return fmt.Errorf("unable to validate server certificate for controller connection: %w", err)
			}
			return nil
		},
	}, nil
}

// CreateRouterTLSConfig creates a TLS config for router connections
func CreateRouterTLSConfig(routerCert *x509.Certificate) (*tls.Config, error) {
	trustStore := NewTrustStore()
	if routerCert != nil {
		trustStore.AddCertificate(routerCert)
	}

	trustManager := trustStore.CreateCombinedTrustManager()

	return &tls.Config{
		RootCAs:            trustStore.systemCerts,
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: false,
		VerifyPeerCertificate: func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
			if err := trustManager.VerifyPeerCertificate(rawCerts, verifiedChains); err != nil {
				return fmt.Errorf("unable to validate server certificate for router connection: %w", err)
			}
			return nil
		},
	}, nil
}

// CreateWebSocketTLSConfig creates a TLS config for websocket connections
func CreateWebSocketTLSConfig(webSocketCert *x509.Certificate) (*tls.Config, error) {
	trustStore := NewTrustStore()
	if webSocketCert != nil {
		trustStore.AddCertificate(webSocketCert)
	}

	trustManager := trustStore.CreateCombinedTrustManager()

	return &tls.Config{
		RootCAs:            trustStore.systemCerts,
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: false,
		VerifyPeerCertificate: func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
			if err := trustManager.VerifyPeerCertificate(rawCerts, verifiedChains); err != nil {
				return fmt.Errorf("unable to validate server certificate for websocket connection: %w", err)
			}
			return nil
		},
	}, nil
}

// CreateInsecureTLSConfig creates a TLS config that skips certificate verification
// This is used when we need to make an insecure connection to get a new certificate
func CreateInsecureTLSConfig() *tls.Config {
	return &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: true, // #nosec G402 -- intentionally insecure for bootstrapping initial certificate fetch
	}
}

// CreateTLSConfigWithClientCerts creates a TLS config with client certificates
func CreateTLSConfigWithClientCerts(caCert *x509.Certificate, tlsCert []byte, tlsKey []byte) (*tls.Config, error) {
	// Load client certificate
	cert, err := tls.X509KeyPair(tlsCert, tlsKey)
	if err != nil {
		return nil, fmt.Errorf("failed to load client certificate: %w", err)
	}

	// Create trust store with CA certificate
	trustStore := NewTrustStore()
	if caCert != nil {
		trustStore.AddCertificate(caCert)
	}

	trustManager := trustStore.CreateCombinedTrustManager()

	return &tls.Config{
		Certificates:       []tls.Certificate{cert},
		RootCAs:            trustStore.systemCerts,
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: false,
		VerifyPeerCertificate: func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
			if err := trustManager.VerifyPeerCertificate(rawCerts, verifiedChains); err != nil {
				return fmt.Errorf("unable to validate server certificate: %w", err)
			}
			return nil
		},
	}, nil
}

// ValidateCertificateChain validates a certificate chain against a trust store
func ValidateCertificateChain(certs []*x509.Certificate, trustStore *TrustStore) error {
	if len(certs) == 0 {
		return errors.New("no certificates in chain")
	}

	// Try system certificates first
	if trustStore.systemCerts != nil {
		opts := x509.VerifyOptions{
			Roots: trustStore.systemCerts,
		}
		_, err := certs[0].Verify(opts)
		if err == nil {
			return nil
		}
	}

	// Try custom certificates
	for _, customCert := range trustStore.customCerts {
		opts := x509.VerifyOptions{
			Roots: x509.NewCertPool(),
		}
		opts.Roots.AddCert(customCert)

		_, err := certs[0].Verify(opts)
		if err == nil {
			return nil
		}
	}

	return errors.New("certificate chain validation failed")
}
