package hf

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
	"unicode"

	"github.com/eclipse-iofog/edgelet/internal/models"
)

func decodeCAB64(caB64 string) ([]byte, error) {
	cleaned := strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, caB64)
	if cleaned == "" {
		return nil, nil
	}
	raw, err := base64.StdEncoding.DecodeString(cleaned)
	if err != nil {
		raw, err = base64.RawStdEncoding.DecodeString(cleaned)
		if err != nil {
			return nil, fmt.Errorf("decode registry ca: %w", err)
		}
	}
	return raw, nil
}

func tlsConfigForRegistry(reg *models.Registry) (*tls.Config, error) {
	cfg := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}
	if reg.Insecure {
		cfg.InsecureSkipVerify = true // #nosec G402 -- controlled by registry insecure flag
	}
	pem, err := decodeCAB64(reg.CAB64)
	if err != nil {
		return nil, err
	}
	if len(pem) == 0 {
		return cfg, nil
	}
	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		pool = x509.NewCertPool()
	}
	if !pool.AppendCertsFromPEM(pem) {
		return nil, errors.New("registry ca is not a valid PEM certificate bundle")
	}
	cfg.RootCAs = pool
	return cfg, nil
}

func httpClientForRegistry(reg *models.Registry) (*http.Client, error) {
	tlsCfg, err := tlsConfigForRegistry(reg)
	if err != nil {
		return nil, err
	}
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout: 10 * time.Second,
		TLSClientConfig:     tlsCfg,
		ForceAttemptHTTP2:   true,
	}
	return &http.Client{Transport: transport}, nil
}
