package models

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func validControlPlaneManifestForTest() *ControlPlaneManifest {
	doc := &ControlPlaneManifest{}
	doc.APIVersion = controlPlaneAPIVersion
	doc.Kind = controlPlaneKind
	doc.Metadata.Name = "pot"
	doc.Metadata.Namespace = "default"
	doc.Spec.Controller.Image = "ghcr.io/datasance/controller:3.7.0"
	return doc
}

func TestControlPlaneManifestValidate_Minimal(t *testing.T) {
	doc := validControlPlaneManifestForTest()
	if err := doc.Validate(); err != nil {
		t.Fatalf("expected minimal manifest to pass: %v", err)
	}
}

func TestControlPlaneManifestValidate_RequiresControllerImage(t *testing.T) {
	doc := validControlPlaneManifestForTest()
	doc.Spec.Controller.Image = ""
	if err := doc.Validate(); err == nil || !strings.Contains(err.Error(), "spec.controller.image is required") {
		t.Fatalf("expected controller image error, got: %v", err)
	}
}

func TestControlPlaneManifestValidate_ForbidsMetadataLabels(t *testing.T) {
	doc := validControlPlaneManifestForTest()
	doc.Metadata.Labels = map[string]string{"team": "edge"}
	if err := doc.Validate(); err == nil || !strings.Contains(err.Error(), "metadata.labels are forbidden") {
		t.Fatalf("expected labels forbidden error, got: %v", err)
	}
}

func TestControlPlaneManifestValidate_RejectsSiteCA(t *testing.T) {
	manifest := `
apiVersion: edgelet.iofog.org/v1
kind: ControlPlane
metadata:
  name: pot
spec:
  controller:
    image: ghcr.io/datasance/controller:3.7.0
  siteCA:
    cert: ignored
`
	_, err := ParseControlPlaneManifest(manifest)
	if err == nil || !strings.Contains(err.Error(), "siteCA") {
		t.Fatalf("expected siteCA rejection, got: %v", err)
	}
}

func TestControlPlaneManifestValidate_RejectsLocalCA(t *testing.T) {
	manifest := `
apiVersion: edgelet.iofog.org/v1
kind: ControlPlane
metadata:
  name: pot
spec:
  controller:
    image: ghcr.io/datasance/controller:3.7.0
  localCA:
    cert: ignored
`
	_, err := ParseControlPlaneManifest(manifest)
	if err == nil || !strings.Contains(err.Error(), "localCA") {
		t.Fatalf("expected localCA rejection, got: %v", err)
	}
}

func TestControlPlaneManifestValidate_HTTPSEmptyBlockAllowed(t *testing.T) {
	doc := validControlPlaneManifestForTest()
	doc.Spec.HTTPS = &struct {
		Path   string `yaml:"path,omitempty" json:"path,omitempty"`
		Base64 *struct {
			CA   string `yaml:"ca,omitempty" json:"ca,omitempty"`
			Cert string `yaml:"cert,omitempty" json:"cert,omitempty"`
			Key  string `yaml:"key,omitempty" json:"key,omitempty"`
		} `yaml:"base64,omitempty" json:"base64,omitempty"`
	}{}
	if err := doc.Validate(); err != nil {
		t.Fatalf("expected empty https block to pass: %v", err)
	}
}

func TestControlPlaneManifestValidate_HTTPSPathAndBase64MutuallyExclusive(t *testing.T) {
	doc := validControlPlaneManifestForTest()
	dir := t.TempDir()
	writeControlPlaneCertFiles(t, dir, false)
	doc.Spec.HTTPS = &struct {
		Path   string `yaml:"path,omitempty" json:"path,omitempty"`
		Base64 *struct {
			CA   string `yaml:"ca,omitempty" json:"ca,omitempty"`
			Cert string `yaml:"cert,omitempty" json:"cert,omitempty"`
			Key  string `yaml:"key,omitempty" json:"key,omitempty"`
		} `yaml:"base64,omitempty" json:"base64,omitempty"`
	}{
		Path: dir,
		Base64: &struct {
			CA   string `yaml:"ca,omitempty" json:"ca,omitempty"`
			Cert string `yaml:"cert,omitempty" json:"cert,omitempty"`
			Key  string `yaml:"key,omitempty" json:"key,omitempty"`
		}{
			Cert: base64.StdEncoding.EncodeToString([]byte("cert")),
			Key:  base64.StdEncoding.EncodeToString([]byte("key")),
		},
	}
	if err := doc.Validate(); err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("expected mutual exclusion error, got: %v", err)
	}
}

func TestControlPlaneManifestValidate_HTTPSPathRequiresAbsoluteExistingDir(t *testing.T) {
	doc := validControlPlaneManifestForTest()
	doc.Spec.HTTPS = &struct {
		Path   string `yaml:"path,omitempty" json:"path,omitempty"`
		Base64 *struct {
			CA   string `yaml:"ca,omitempty" json:"ca,omitempty"`
			Cert string `yaml:"cert,omitempty" json:"cert,omitempty"`
			Key  string `yaml:"key,omitempty" json:"key,omitempty"`
		} `yaml:"base64,omitempty" json:"base64,omitempty"`
	}{Path: "relative/certs"}
	if err := doc.Validate(); err == nil || !strings.Contains(err.Error(), "absolute") {
		t.Fatalf("expected absolute path error, got: %v", err)
	}

	dir := t.TempDir()
	doc.Spec.HTTPS.Path = dir
	if err := doc.Validate(); err == nil || !strings.Contains(err.Error(), "tls.crt") {
		t.Fatalf("expected missing tls.crt error, got: %v", err)
	}

	writeControlPlaneCertFiles(t, dir, true)
	if err := doc.Validate(); err != nil {
		t.Fatalf("expected valid https path to pass: %v", err)
	}
}

func TestControlPlaneManifestValidate_HTTPSBase64RequiresValidEncoding(t *testing.T) {
	doc := validControlPlaneManifestForTest()
	doc.Spec.HTTPS = &struct {
		Path   string `yaml:"path,omitempty" json:"path,omitempty"`
		Base64 *struct {
			CA   string `yaml:"ca,omitempty" json:"ca,omitempty"`
			Cert string `yaml:"cert,omitempty" json:"cert,omitempty"`
			Key  string `yaml:"key,omitempty" json:"key,omitempty"`
		} `yaml:"base64,omitempty" json:"base64,omitempty"`
	}{
		Base64: &struct {
			CA   string `yaml:"ca,omitempty" json:"ca,omitempty"`
			Cert string `yaml:"cert,omitempty" json:"cert,omitempty"`
			Key  string `yaml:"key,omitempty" json:"key,omitempty"`
		}{
			Cert: "not-base64!!!",
			Key:  base64.StdEncoding.EncodeToString([]byte("key")),
		},
	}
	if err := doc.Validate(); err == nil || !strings.Contains(err.Error(), "base64") {
		t.Fatalf("expected base64 validation error, got: %v", err)
	}
}

func TestControlPlaneManifestNormalizeDefaults(t *testing.T) {
	doc := validControlPlaneManifestForTest()
	doc.Metadata.Namespace = ""
	doc.NormalizeDefaults()
	if doc.Metadata.Namespace != ControlPlaneDefaultNamespace {
		t.Fatalf("expected namespace default %q, got %q", ControlPlaneDefaultNamespace, doc.Metadata.Namespace)
	}
}

func writeControlPlaneCertFiles(t *testing.T, dir string, withCA bool) {
	t.Helper()
	for _, name := range []string{ControlPlaneHTTPSCertFilename, ControlPlaneHTTPSKeyFilename} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("pem"), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	if withCA {
		if err := os.WriteFile(filepath.Join(dir, ControlPlaneHTTPSCAFilename), []byte("ca"), 0o600); err != nil {
			t.Fatalf("write ca: %v", err)
		}
	}
}
