package oci

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"

	"github.com/eclipse-iofog/edgelet/internal/models"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/retry"
)

// registryEndpoint is the HTTP identity of an OCI registry.
type registryEndpoint struct {
	Host      string
	Scheme    string
	PlainHTTP bool
	RepoRef   string // host/repo for oras NewRepository
}

func parseRegistryEndpoint(reg *models.Registry, repo string) (registryEndpoint, error) {
	if reg == nil {
		return registryEndpoint{}, errors.New("registry is required")
	}
	repo = strings.TrimSpace(repo)
	if repo == "" {
		return registryEndpoint{}, errors.New("repo is required")
	}
	raw := strings.TrimSpace(reg.URL)
	if raw == "" {
		return registryEndpoint{}, errors.New("registry url is required")
	}

	scheme := "https"
	host := raw
	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err != nil {
			return registryEndpoint{}, fmt.Errorf("parse registry url: %w", err)
		}
		if u.Host == "" {
			return registryEndpoint{}, fmt.Errorf("registry url %q has no host", raw)
		}
		// Path on the registry URL is ignored; spec.repo is the repository path.
		scheme = strings.ToLower(u.Scheme)
		host = u.Host
	}
	host = strings.TrimSuffix(host, "/")

	plainHTTP := false
	switch scheme {
	case "http":
		if !reg.Insecure {
			return registryEndpoint{}, errors.New("http registry urls require insecure=true")
		}
		plainHTTP = true
	case "https", "":
		scheme = "https"
	default:
		return registryEndpoint{}, fmt.Errorf("unsupported registry url scheme %q", scheme)
	}

	return registryEndpoint{
		Host:      host,
		Scheme:    scheme,
		PlainHTTP: plainHTTP,
		RepoRef:   host + "/" + strings.TrimPrefix(repo, "/"),
	}, nil
}

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
	base := retry.DefaultClient
	client := *base
	client.Transport = retry.NewTransport(transport)
	return &client, nil
}

func newRemoteRepository(reg *models.Registry, repo string, httpClient *http.Client) (*remote.Repository, *auth.Client, registryEndpoint, error) {
	ep, err := parseRegistryEndpoint(reg, repo)
	if err != nil {
		return nil, nil, registryEndpoint{}, err
	}
	if httpClient == nil {
		httpClient, err = httpClientForRegistry(reg)
		if err != nil {
			return nil, nil, registryEndpoint{}, err
		}
	}
	authClient := &auth.Client{
		Client: httpClient,
		Cache:  auth.NewCache(),
		Header: http.Header{
			"User-Agent": {"edgelet-model-pull"},
		},
	}
	if !reg.IsPublic && (strings.TrimSpace(reg.UserName) != "" || strings.TrimSpace(reg.Password) != "") {
		authClient.Credential = auth.StaticCredential(ep.Host, auth.Credential{
			Username: reg.UserName,
			Password: reg.Password,
		})
	}
	remoteRepo, err := remote.NewRepository(ep.RepoRef)
	if err != nil {
		return nil, nil, registryEndpoint{}, fmt.Errorf("open registry repository: %w", err)
	}
	remoteRepo.Client = authClient
	remoteRepo.PlainHTTP = ep.PlainHTTP
	return remoteRepo, authClient, ep, nil
}
