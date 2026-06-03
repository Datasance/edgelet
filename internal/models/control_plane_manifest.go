package models

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	controlPlaneKind       = "ControlPlane"
	controlPlaneAPIVersion = "edgelet.iofog.org/v1"
	// ControlPlaneHTTPS*Filename are container-relative names under /etc/iofog/controller-cert/.
	ControlPlaneHTTPSCertFilename = "tls.crt"
	ControlPlaneHTTPSKeyFilename  = "tls.key"
	ControlPlaneHTTPSCAFilename   = "ca.crt"
)

// ControlPlaneManifest is the operator YAML for kind ControlPlane.
type ControlPlaneManifest struct {
	APIVersion string `yaml:"apiVersion" json:"apiVersion"`
	Kind       string `yaml:"kind" json:"kind"`
	Metadata   struct {
		Name      string            `yaml:"name" json:"name"`
		Namespace string            `yaml:"namespace,omitempty" json:"namespace,omitempty"`
		Labels    map[string]string `yaml:"labels,omitempty" json:"labels,omitempty"`
	} `yaml:"metadata" json:"metadata"`
	Spec ControlPlaneManifestSpec `yaml:"spec" json:"spec"`
}

// ControlPlaneManifestSpec holds controller deployment configuration.
type ControlPlaneManifestSpec struct {
	Controller struct {
		Image    string `yaml:"image" json:"image"`
		Registry *int   `yaml:"registry,omitempty" json:"registry,omitempty"`
		Port     *int   `yaml:"port,omitempty" json:"port,omitempty"`
	} `yaml:"controller" json:"controller"`
	Auth struct {
		URL              string `yaml:"url,omitempty" json:"url,omitempty"`
		Realm            string `yaml:"realm,omitempty" json:"realm,omitempty"`
		RealmKey         string `yaml:"realmKey,omitempty" json:"realmKey,omitempty"`
		SSL              string `yaml:"ssl,omitempty" json:"ssl,omitempty"`
		ControllerClient string `yaml:"controllerClient,omitempty" json:"controllerClient,omitempty"`
		ControllerSecret string `yaml:"controllerSecret,omitempty" json:"controllerSecret,omitempty"`
		ViewerClient     string `yaml:"viewerClient,omitempty" json:"viewerClient,omitempty"`
	} `yaml:"auth,omitempty" json:"auth,omitempty"`
	Database *struct {
		Provider     string `yaml:"provider,omitempty" json:"provider,omitempty"`
		User         string `yaml:"user,omitempty" json:"user,omitempty"`
		Host         string `yaml:"host,omitempty" json:"host,omitempty"`
		Port         *int   `yaml:"port,omitempty" json:"port,omitempty"`
		Password     string `yaml:"password,omitempty" json:"password,omitempty"`
		DatabaseName string `yaml:"databaseName,omitempty" json:"databaseName,omitempty"`
		SSL          *bool  `yaml:"ssl,omitempty" json:"ssl,omitempty"`
		CA           string `yaml:"ca,omitempty" json:"ca,omitempty"`
	} `yaml:"database,omitempty" json:"database,omitempty"`
	Events *struct {
		AuditEnabled     *bool `yaml:"auditEnabled,omitempty" json:"auditEnabled,omitempty"`
		RetentionDays    *int  `yaml:"retentionDays,omitempty" json:"retentionDays,omitempty"`
		CleanupInterval  *int  `yaml:"cleanupInterval,omitempty" json:"cleanupInterval,omitempty"`
		CaptureIPAddress *bool `yaml:"captureIpAddress,omitempty" json:"captureIpAddress,omitempty"`
	} `yaml:"events,omitempty" json:"events,omitempty"`
	SystemMicroservices *struct {
		Router map[string]string `yaml:"router,omitempty" json:"router,omitempty"`
		NATS   map[string]string `yaml:"nats,omitempty" json:"nats,omitempty"`
	} `yaml:"systemMicroservices,omitempty" json:"systemMicroservices,omitempty"`
	NATS *struct {
		Enabled *bool `yaml:"enabled,omitempty" json:"enabled,omitempty"`
	} `yaml:"nats,omitempty" json:"nats,omitempty"`
	ECNViewerPort *int   `yaml:"ecnViewerPort,omitempty" json:"ecnViewerPort,omitempty"`
	ECNViewerURL  string `yaml:"ecnViewerUrl,omitempty" json:"ecnViewerUrl,omitempty"`
	LogLevel      string `yaml:"logLevel,omitempty" json:"logLevel,omitempty"`
	HTTPS         *struct {
		Path   string `yaml:"path,omitempty" json:"path,omitempty"`
		Base64 *struct {
			CA   string `yaml:"ca,omitempty" json:"ca,omitempty"`
			Cert string `yaml:"cert,omitempty" json:"cert,omitempty"`
			Key  string `yaml:"key,omitempty" json:"key,omitempty"`
		} `yaml:"base64,omitempty" json:"base64,omitempty"`
	} `yaml:"https,omitempty" json:"https,omitempty"`
	Vault *ControlPlaneVaultSpec `yaml:"vault,omitempty" json:"vault,omitempty"`
	// Forbidden in Edgelet ControlPlane YAML (potctl REST import only).
	SiteCA  any `yaml:"siteCA,omitempty" json:"-"`
	LocalCA any `yaml:"localCA,omitempty" json:"-"`
}

// ControlPlaneVaultSpec mirrors optional vault provider blocks.
type ControlPlaneVaultSpec struct {
	Enabled   *bool  `yaml:"enabled,omitempty" json:"enabled,omitempty"`
	Provider  string `yaml:"provider,omitempty" json:"provider,omitempty"`
	BasePath  string `yaml:"basePath,omitempty" json:"basePath,omitempty"`
	Hashicorp *struct {
		Address string `yaml:"address,omitempty" json:"address,omitempty"`
		Token   string `yaml:"token,omitempty" json:"token,omitempty"`
		Mount   string `yaml:"mount,omitempty" json:"mount,omitempty"`
	} `yaml:"hashicorp,omitempty" json:"hashicorp,omitempty"`
	AWS *struct {
		Region      string `yaml:"region,omitempty" json:"region,omitempty"`
		AccessKeyID string `yaml:"accessKeyId,omitempty" json:"accessKeyId,omitempty"`
		AccessKey   string `yaml:"accessKey,omitempty" json:"accessKey,omitempty"`
	} `yaml:"aws,omitempty" json:"aws,omitempty"`
	Azure *struct {
		URL          string `yaml:"url,omitempty" json:"url,omitempty"`
		TenantID     string `yaml:"tenantId,omitempty" json:"tenantId,omitempty"`
		ClientID     string `yaml:"clientId,omitempty" json:"clientId,omitempty"`
		ClientSecret string `yaml:"clientSecret,omitempty" json:"clientSecret,omitempty"`
	} `yaml:"azure,omitempty" json:"azure,omitempty"`
	Google *struct {
		ProjectID   string `yaml:"projectId,omitempty" json:"projectId,omitempty"`
		Credentials string `yaml:"credentials,omitempty" json:"credentials,omitempty"`
	} `yaml:"google,omitempty" json:"google,omitempty"`
}

// ParseControlPlaneManifest decodes and validates ControlPlane YAML.
func ParseControlPlaneManifest(manifest string) (*ControlPlaneManifest, error) {
	doc := &ControlPlaneManifest{}
	dec := yaml.NewDecoder(bytes.NewReader([]byte(strings.TrimSpace(manifest))))
	dec.KnownFields(true)
	if err := dec.Decode(doc); err != nil {
		return nil, fmt.Errorf("invalid manifest YAML: %w", err)
	}
	if err := doc.Validate(); err != nil {
		return nil, err
	}
	return doc, nil
}

// NormalizeDefaults applies metadata defaults before validation or env build.
func (m *ControlPlaneManifest) NormalizeDefaults() {
	if m == nil {
		return
	}
	if strings.TrimSpace(m.Metadata.Namespace) == "" {
		m.Metadata.Namespace = ControlPlaneDefaultNamespace
	}
}

// Validate checks ControlPlane manifest semantics.
func (m *ControlPlaneManifest) Validate() error {
	if m == nil {
		return fmt.Errorf("manifest is nil")
	}
	if strings.TrimSpace(m.APIVersion) == "" {
		return fmt.Errorf("apiVersion is required")
	}
	if strings.TrimSpace(m.APIVersion) != controlPlaneAPIVersion {
		return fmt.Errorf("apiVersion must be %s", controlPlaneAPIVersion)
	}
	if strings.TrimSpace(m.Kind) == "" {
		return fmt.Errorf("kind is required")
	}
	if strings.TrimSpace(m.Kind) != controlPlaneKind {
		return fmt.Errorf("kind must be %s", controlPlaneKind)
	}
	if len(m.Metadata.Labels) > 0 {
		return fmt.Errorf("metadata.labels are forbidden on ControlPlane manifests")
	}
	name := strings.TrimSpace(m.Metadata.Name)
	if name == "" {
		return fmt.Errorf("metadata.name is required")
	}
	if len(name) > 63 || !localDeployNamePattern.MatchString(name) {
		return fmt.Errorf("metadata.name must be <= 63 characters and follow DNS-1123 label format")
	}
	ns := strings.TrimSpace(m.Metadata.Namespace)
	if ns != "" {
		if len(ns) > 63 || !localDeployNamePattern.MatchString(ns) {
			return fmt.Errorf("metadata.namespace must be <= 63 characters and follow DNS-1123 label format")
		}
	}
	if m.Spec.SiteCA != nil {
		return fmt.Errorf("spec.siteCA is not supported in ControlPlane manifests; import via Controller REST API after deploy")
	}
	if m.Spec.LocalCA != nil {
		return fmt.Errorf("spec.localCA is not supported in ControlPlane manifests; import via Controller REST API after deploy")
	}
	if strings.TrimSpace(m.Spec.Controller.Image) == "" {
		return fmt.Errorf("spec.controller.image is required")
	}
	if m.Spec.Controller.Port != nil && *m.Spec.Controller.Port <= 0 {
		return fmt.Errorf("spec.controller.port must be positive when set")
	}
	if m.Spec.ECNViewerPort != nil && *m.Spec.ECNViewerPort <= 0 {
		return fmt.Errorf("spec.ecnViewerPort must be positive when set")
	}
	if err := validateControlPlaneHTTPS(m.Spec.HTTPS); err != nil {
		return err
	}
	return nil
}

func validateControlPlaneHTTPS(https *struct {
	Path   string `yaml:"path,omitempty" json:"path,omitempty"`
	Base64 *struct {
		CA   string `yaml:"ca,omitempty" json:"ca,omitempty"`
		Cert string `yaml:"cert,omitempty" json:"cert,omitempty"`
		Key  string `yaml:"key,omitempty" json:"key,omitempty"`
	} `yaml:"base64,omitempty" json:"base64,omitempty"`
}) error {
	if https == nil {
		return nil
	}
	path := strings.TrimSpace(https.Path)
	hasPath := path != ""
	hasBase64 := https.Base64 != nil && (strings.TrimSpace(https.Base64.Cert) != "" ||
		strings.TrimSpace(https.Base64.Key) != "" ||
		strings.TrimSpace(https.Base64.CA) != "")
	if hasPath && hasBase64 {
		return fmt.Errorf("spec.https.path and spec.https.base64 are mutually exclusive")
	}
	if !hasPath && !hasBase64 {
		return nil
	}
	if hasPath {
		if !isValidHostPath(path) {
			return fmt.Errorf("spec.https.path must be an absolute host path")
		}
		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("spec.https.path must exist on the Edgelet host: %w", err)
		}
		if !info.IsDir() {
			return fmt.Errorf("spec.https.path must be a directory")
		}
		for _, file := range []string{ControlPlaneHTTPSCertFilename, ControlPlaneHTTPSKeyFilename} {
			if _, err := os.Stat(filepath.Join(path, file)); err != nil {
				return fmt.Errorf("spec.https.path must contain %s: %w", file, err)
			}
		}
		return nil
	}
	b := https.Base64
	if strings.TrimSpace(b.Cert) == "" {
		return fmt.Errorf("spec.https.base64.cert is required when spec.https.base64 is set")
	}
	if strings.TrimSpace(b.Key) == "" {
		return fmt.Errorf("spec.https.base64.key is required when spec.https.base64 is set")
	}
	for _, item := range []struct {
		field string
		value string
	}{
		{"cert", b.Cert},
		{"key", b.Key},
		{"ca", b.CA},
	} {
		if strings.TrimSpace(item.value) == "" {
			continue
		}
		if _, err := base64.StdEncoding.DecodeString(strings.TrimSpace(item.value)); err != nil {
			return fmt.Errorf("spec.https.base64.%s must be valid base64: %w", item.field, err)
		}
	}
	return nil
}

// ManifestControllerImage returns spec.controller.image trimmed.
func (m *ControlPlaneManifest) ManifestControllerImage() string {
	if m == nil {
		return ""
	}
	return strings.TrimSpace(m.Spec.Controller.Image)
}

// ControllerRegistryID returns spec.controller.registry when set.
func (m *ControlPlaneManifest) ControllerRegistryID() (int, bool) {
	if m == nil || m.Spec.Controller.Registry == nil {
		return 0, false
	}
	return *m.Spec.Controller.Registry, true
}
