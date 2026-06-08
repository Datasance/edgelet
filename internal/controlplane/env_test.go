//revive:disable:nested-structs
package controlplane

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	"github.com/datasance/edgelet/internal/models"
)

func TestBuildControllerEnv_MinimalRemoteIdentity(t *testing.T) {
	doc := &models.ControlPlaneManifest{}
	doc.APIVersion = "edgelet.iofog.org/v1"
	doc.Kind = "ControlPlane"
	doc.Metadata.Name = "pot"
	doc.Spec.Controller.Image = "ghcr.io/datasance/controller:3.7.0"

	env, err := BuildControllerEnv(doc, "uuid-1")
	if err != nil {
		t.Fatalf("BuildControllerEnv: %v", err)
	}
	if env["CONTROL_PLANE"] != "Remote" {
		t.Fatalf("CONTROL_PLANE=%q, want Remote", env["CONTROL_PLANE"])
	}
	if env["CONTROLLER_UUID"] != "uuid-1" {
		t.Fatalf("CONTROLLER_UUID=%q", env["CONTROLLER_UUID"])
	}
	if env["CONTROLLER_NAMESPACE"] != models.ControlPlaneDefaultNamespace {
		t.Fatalf("CONTROLLER_NAMESPACE=%q", env["CONTROLLER_NAMESPACE"])
	}
	if env["CONTROLLER_NAME"] != "pot" {
		t.Fatalf("CONTROLLER_NAME=%q", env["CONTROLLER_NAME"])
	}
	if _, ok := env["SERVER_PORT"]; ok {
		t.Fatal("expected SERVER_PORT omitted when port unset")
	}
}

func TestBuildControllerEnv_FullProjection(t *testing.T) {
	port := 51121
	viewerPort := 8008
	dbPort := 5432
	dbSSL := true
	audit := true
	retention := 14
	cleanup := 86400
	captureIP := false
	natsEnabled := true
	vaultEnabled := true
	registry := 1

	doc := &models.ControlPlaneManifest{}
	doc.APIVersion = "edgelet.iofog.org/v1"
	doc.Kind = "ControlPlane"
	doc.Metadata.Name = "pot"
	doc.Metadata.Namespace = "cp-ns"
	doc.Spec.Controller.Image = "ghcr.io/datasance/controller:3.7.0"
	doc.Spec.Controller.Registry = &registry
	doc.Spec.Controller.Port = &port
	doc.Spec.ECNViewerPort = &viewerPort
	doc.Spec.ECNViewerURL = "https://viewer.example"
	doc.Spec.LogLevel = "info"
	doc.Spec.Auth.URL = "https://auth.example/"
	doc.Spec.Auth.Realm = "realm"
	doc.Spec.Auth.RealmKey = "key"
	doc.Spec.Auth.SSL = "external"
	doc.Spec.Auth.ControllerClient = "pot-controller"
	doc.Spec.Auth.ControllerSecret = "secret"
	doc.Spec.Auth.ViewerClient = "ecn-viewer"
	doc.Spec.Database = &struct {
		Provider     string `yaml:"provider,omitempty" json:"provider,omitempty"`
		User         string `yaml:"user,omitempty" json:"user,omitempty"`
		Host         string `yaml:"host,omitempty" json:"host,omitempty"`
		Port         *int   `yaml:"port,omitempty" json:"port,omitempty"`
		Password     string `yaml:"password,omitempty" json:"password,omitempty"`
		DatabaseName string `yaml:"databaseName,omitempty" json:"databaseName,omitempty"`
		SSL          *bool  `yaml:"ssl,omitempty" json:"ssl,omitempty"`
		CA           string `yaml:"ca,omitempty" json:"ca,omitempty"`
	}{
		Provider:     "postgres",
		User:         "cp",
		Host:         "db.local",
		Port:         &dbPort,
		Password:     "pw",
		DatabaseName: "controller",
		SSL:          &dbSSL,
		CA:           "ca-b64",
	}
	doc.Spec.Events = &struct {
		AuditEnabled     *bool `yaml:"auditEnabled,omitempty" json:"auditEnabled,omitempty"`
		RetentionDays    *int  `yaml:"retentionDays,omitempty" json:"retentionDays,omitempty"`
		CleanupInterval  *int  `yaml:"cleanupInterval,omitempty" json:"cleanupInterval,omitempty"`
		CaptureIPAddress *bool `yaml:"captureIpAddress,omitempty" json:"captureIpAddress,omitempty"`
	}{
		AuditEnabled:     &audit,
		RetentionDays:    &retention,
		CleanupInterval:  &cleanup,
		CaptureIPAddress: &captureIP,
	}
	doc.Spec.SystemMicroservices = &struct {
		Router map[string]string `yaml:"router,omitempty" json:"router,omitempty"`
		NATS   map[string]string `yaml:"nats,omitempty" json:"nats,omitempty"`
	}{
		Router: map[string]string{
			"amd64":   "router-amd64",
			"arm64":   "router-arm64",
			"riscv64": "router-riscv",
			"arm":     "router-arm",
		},
		NATS: map[string]string{
			"amd64": "nats-amd64",
			"arm":   "nats-arm",
		},
	}
	doc.Spec.NATS = &struct {
		Enabled *bool `yaml:"enabled,omitempty" json:"enabled,omitempty"`
	}{Enabled: &natsEnabled}
	doc.Spec.Vault = &models.ControlPlaneVaultSpec{
		Enabled:  &vaultEnabled,
		Provider: "hashicorp",
		BasePath: "pot/secrets",
		Hashicorp: &struct {
			Address string `yaml:"address,omitempty" json:"address,omitempty"`
			Token   string `yaml:"token,omitempty" json:"token,omitempty"`
			Mount   string `yaml:"mount,omitempty" json:"mount,omitempty"`
		}{
			Address: "http://vault:8200",
			Token:   "token",
			Mount:   "kv",
		},
	}
	cert := base64.StdEncoding.EncodeToString([]byte("cert"))
	key := base64.StdEncoding.EncodeToString([]byte("key"))
	doc.Spec.HTTPS = &models.ControlPlaneHTTPSConfig{
		Base64: &models.ControlPlaneHTTPSBase64{Cert: cert, Key: key},
	}

	env, err := BuildControllerEnv(doc, "uuid-full")
	if err != nil {
		t.Fatalf("BuildControllerEnv: %v", err)
	}

	assertEnv(t, env, "SERVER_PORT", "51121")
	assertEnv(t, env, "VIEWER_PORT", "8008")
	assertEnv(t, env, "VIEWER_URL", "https://viewer.example")
	assertEnv(t, env, "LOG_LEVEL", "info")
	assertEnv(t, env, "KC_URL", "https://auth.example/")
	assertEnv(t, env, "KC_REALM", "realm")
	assertEnv(t, env, "KC_CLIENT", "pot-controller")
	assertEnv(t, env, "DB_PROVIDER", "postgres")
	assertEnv(t, env, "DB_PORT", "5432")
	assertEnv(t, env, "DB_USE_SSL", "true")
	assertEnv(t, env, "EVENT_AUDIT_ENABLED", "true")
	assertEnv(t, env, "EVENT_RETENTION_DAYS", "14")
	assertEnv(t, env, "ROUTER_IMAGE_1", "router-amd64")
	assertEnv(t, env, "ROUTER_IMAGE_2", "router-arm64")
	assertEnv(t, env, "ROUTER_IMAGE_3", "router-riscv")
	assertEnv(t, env, "ROUTER_IMAGE_4", "router-arm")
	assertEnv(t, env, "NATS_IMAGE_1", "nats-amd64")
	assertEnv(t, env, "NATS_IMAGE_4", "nats-arm")
	if _, ok := env["NATS_IMAGE_2"]; ok {
		t.Fatal("expected NATS_IMAGE_2 omitted")
	}
	assertEnv(t, env, "NATS_ENABLED", "true")
	assertEnv(t, env, "VAULT_ENABLED", "true")
	assertEnv(t, env, "VAULT_HASHICORP_ADDRESS", "http://vault:8200")
	assertEnv(t, env, "SSL_BASE64_CERT", cert)
	assertEnv(t, env, "SSL_BASE64_KEY", key)
}

func TestBuildControllerEnv_HTTPSPathFilenames(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{models.ControlPlaneHTTPSCertFilename, models.ControlPlaneHTTPSKeyFilename, models.ControlPlaneHTTPSCAFilename} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	doc := &models.ControlPlaneManifest{}
	doc.APIVersion = "edgelet.iofog.org/v1"
	doc.Kind = "ControlPlane"
	doc.Metadata.Name = "pot"
	doc.Spec.Controller.Image = "ghcr.io/datasance/controller:3.7.0"
	doc.Spec.HTTPS = &models.ControlPlaneHTTPSConfig{Path: dir}

	env, err := BuildControllerEnv(doc, "uuid-tls")
	if err != nil {
		t.Fatalf("BuildControllerEnv: %v", err)
	}
	assertEnv(t, env, "SSL_PATH_CERT", models.ControlPlaneHTTPSCertFilename)
	assertEnv(t, env, "SSL_PATH_KEY", models.ControlPlaneHTTPSKeyFilename)
	assertEnv(t, env, "SSL_PATH_INTERMEDIATE_CERT", models.ControlPlaneHTTPSCAFilename)
}

func assertEnv(t *testing.T, env map[string]string, key, want string) {
	t.Helper()
	if env[key] != want {
		t.Fatalf("%s=%q, want %q", key, env[key], want)
	}
}
