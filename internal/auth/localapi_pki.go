package auth

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/eclipse-iofog/agent/internal/utils"
)

const (
	localAPICACertFilename     = "localapi-ca.crt"
	localAPIServerCertFilename = "localapi-server.crt"
	localAPIServerKeyFilename  = "localapi-server.key"
	localAPIServerDNSName      = "iofog.default.svc.bridge.local"
)

func LocalAPIPKIPaths() (caPath, certPath, keyPath string) {
	base := utils.GetConfigDir()
	return filepath.Join(base, localAPICACertFilename), filepath.Join(base, localAPIServerCertFilename), filepath.Join(base, localAPIServerKeyFilename)
}

func EnsureLocalAPIPKI() (string, string, string, error) {
	caPath, certPath, keyPath := LocalAPIPKIPaths()
	if err := os.MkdirAll(filepath.Dir(caPath), 0700); err != nil {
		return "", "", "", fmt.Errorf("failed to ensure localapi pki directory: %w", err)
	}
	if fileReadable(caPath) && fileReadable(certPath) && fileReadable(keyPath) {
		return caPath, certPath, keyPath, nil
	}
	if err := generateLocalAPIPKI(caPath, certPath, keyPath); err != nil {
		return "", "", "", err
	}
	return caPath, certPath, keyPath, nil
}

func ReadLocalAPICACertPEM() ([]byte, error) {
	caPath, _, _, err := EnsureLocalAPIPKI()
	if err != nil {
		return nil, err
	}
	return os.ReadFile(caPath) // #nosec G304
}

func fileReadable(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func generateLocalAPIPKI(caPath, certPath, keyPath string) error {
	caPub, caPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return fmt.Errorf("failed to generate localapi ca key: %w", err)
	}
	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject: pkix.Name{
			CommonName:   "iofog-agent-localapi-ca",
			Organization: []string{"iofog-agent"},
		},
		NotBefore:             time.Now().Add(-5 * time.Minute),
		NotAfter:              time.Now().Add(10 * 365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, caPub, caPriv)
	if err != nil {
		return fmt.Errorf("failed to create localapi ca cert: %w", err)
	}

	srvPub, srvPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return fmt.Errorf("failed to generate localapi server key: %w", err)
	}
	srvTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano() + 1),
		Subject: pkix.Name{
			CommonName:   localAPIServerDNSName,
			Organization: []string{"iofog-agent"},
		},
		NotBefore:             time.Now().Add(-5 * time.Minute),
		NotAfter:              time.Now().Add(3 * 365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{localAPIServerDNSName, "localhost"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}
	caParsed, err := x509.ParseCertificate(caDER)
	if err != nil {
		return fmt.Errorf("failed to parse localapi ca cert: %w", err)
	}
	srvDER, err := x509.CreateCertificate(rand.Reader, srvTemplate, caParsed, srvPub, caPriv)
	if err != nil {
		return fmt.Errorf("failed to create localapi server cert: %w", err)
	}

	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})
	srvPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srvDER})
	keyPEM, err := x509.MarshalPKCS8PrivateKey(srvPriv)
	if err != nil {
		return fmt.Errorf("failed to marshal localapi server key: %w", err)
	}
	srvKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyPEM})

	if err := writeFileAtomically(caPath, caPEM, 0644); err != nil {
		return err
	}
	if err := writeFileAtomically(certPath, srvPEM, 0644); err != nil {
		return err
	}
	if err := writeFileAtomically(keyPath, srvKeyPEM, 0600); err != nil {
		return err
	}
	return nil
}

func writeFileAtomically(path string, data []byte, mode os.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, mode); err != nil {
		return fmt.Errorf("failed to write temp pki file %s: %w", path, err)
	}
	if err := os.Chmod(tmp, mode); err != nil {
		return fmt.Errorf("failed to chmod temp pki file %s: %w", path, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("failed to activate pki file %s: %w", path, err)
	}
	return nil
}
