//revive:disable:nested-structs
package controlplane

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/datasance/edgelet/internal/models"
)

const controlPlaneRemote = "Remote"

var controlPlaneRouterArchSlots = []struct {
	arch string
	slot int
}{
	{"amd64", 1},
	{"arm64", 2},
	{"riscv64", 3},
	{"arm", 4},
}

// BuildControllerEnv projects a validated ControlPlane manifest into Controller container env.
// Omitted spec blocks emit no variables; CONTROL_PLANE=Remote is always set.
func BuildControllerEnv(doc *models.ControlPlaneManifest, controllerUUID string) (map[string]string, error) {
	if doc == nil {
		return nil, errors.New("manifest is nil")
	}
	if strings.TrimSpace(controllerUUID) == "" {
		return nil, errors.New("controllerUUID is required")
	}
	doc.NormalizeDefaults()
	if err := doc.Validate(); err != nil {
		return nil, err
	}

	env := map[string]string{
		"CONTROL_PLANE":        controlPlaneRemote,
		"CONTROLLER_UUID":      strings.TrimSpace(controllerUUID),
		"CONTROLLER_NAMESPACE": strings.TrimSpace(doc.Metadata.Namespace),
		"CONTROLLER_NAME":      strings.TrimSpace(doc.Metadata.Name),
	}

	if doc.Spec.Controller.Port != nil {
		setEnv(env, "SERVER_PORT", strconv.Itoa(*doc.Spec.Controller.Port))
	}
	if doc.Spec.ECNViewerPort != nil {
		setEnv(env, "VIEWER_PORT", strconv.Itoa(*doc.Spec.ECNViewerPort))
	}
	setEnvIfNonEmpty(env, "VIEWER_URL", doc.Spec.ECNViewerURL)
	setEnvIfNonEmpty(env, "LOG_LEVEL", doc.Spec.LogLevel)

	projectAuthEnv(env, &doc.Spec.Auth)
	projectDatabaseEnv(env, doc.Spec.Database)
	projectEventsEnv(env, doc.Spec.Events)
	projectSystemMicroserviceEnv(env, doc.Spec.SystemMicroservices)
	projectNATSEnv(env, doc.Spec.NATS)
	projectHTTPSEnv(env, doc.Spec.HTTPS)
	projectVaultEnv(env, doc.Spec.Vault)

	return env, nil
}

func projectAuthEnv(env map[string]string, auth *struct {
	URL              string `yaml:"url,omitempty" json:"url,omitempty"`
	Realm            string `yaml:"realm,omitempty" json:"realm,omitempty"`
	RealmKey         string `yaml:"realmKey,omitempty" json:"realmKey,omitempty"`
	SSL              string `yaml:"ssl,omitempty" json:"ssl,omitempty"`
	ControllerClient string `yaml:"controllerClient,omitempty" json:"controllerClient,omitempty"`
	ControllerSecret string `yaml:"controllerSecret,omitempty" json:"controllerSecret,omitempty"`
	ViewerClient     string `yaml:"viewerClient,omitempty" json:"viewerClient,omitempty"`
}) {
	setEnvIfNonEmpty(env, "KC_URL", auth.URL)
	setEnvIfNonEmpty(env, "KC_REALM", auth.Realm)
	setEnvIfNonEmpty(env, "KC_REALM_KEY", auth.RealmKey)
	setEnvIfNonEmpty(env, "KC_SSL_REQ", auth.SSL)
	setEnvIfNonEmpty(env, "KC_CLIENT", auth.ControllerClient)
	setEnvIfNonEmpty(env, "KC_CLIENT_SECRET", auth.ControllerSecret)
	setEnvIfNonEmpty(env, "KC_VIEWER_CLIENT", auth.ViewerClient)
}

func projectDatabaseEnv(env map[string]string, db *struct {
	Provider     string `yaml:"provider,omitempty" json:"provider,omitempty"`
	User         string `yaml:"user,omitempty" json:"user,omitempty"`
	Host         string `yaml:"host,omitempty" json:"host,omitempty"`
	Port         *int   `yaml:"port,omitempty" json:"port,omitempty"`
	Password     string `yaml:"password,omitempty" json:"password,omitempty"`
	DatabaseName string `yaml:"databaseName,omitempty" json:"databaseName,omitempty"`
	SSL          *bool  `yaml:"ssl,omitempty" json:"ssl,omitempty"`
	CA           string `yaml:"ca,omitempty" json:"ca,omitempty"`
}) {
	if db == nil {
		return
	}
	setEnvIfNonEmpty(env, "DB_PROVIDER", db.Provider)
	setEnvIfNonEmpty(env, "DB_USERNAME", db.User)
	setEnvIfNonEmpty(env, "DB_HOST", db.Host)
	if db.Port != nil {
		setEnv(env, "DB_PORT", strconv.Itoa(*db.Port))
	}
	setEnvIfNonEmpty(env, "DB_PASSWORD", db.Password)
	setEnvIfNonEmpty(env, "DB_NAME", db.DatabaseName)
	if db.SSL != nil {
		setEnv(env, "DB_USE_SSL", strconv.FormatBool(*db.SSL))
	}
	setEnvIfNonEmpty(env, "DB_SSL_CA", db.CA)
}

func projectEventsEnv(env map[string]string, events *struct {
	AuditEnabled     *bool `yaml:"auditEnabled,omitempty" json:"auditEnabled,omitempty"`
	RetentionDays    *int  `yaml:"retentionDays,omitempty" json:"retentionDays,omitempty"`
	CleanupInterval  *int  `yaml:"cleanupInterval,omitempty" json:"cleanupInterval,omitempty"`
	CaptureIPAddress *bool `yaml:"captureIpAddress,omitempty" json:"captureIpAddress,omitempty"`
}) {
	if events == nil {
		return
	}
	if events.AuditEnabled != nil {
		setEnv(env, "EVENT_AUDIT_ENABLED", strconv.FormatBool(*events.AuditEnabled))
	}
	if events.RetentionDays != nil {
		setEnv(env, "EVENT_RETENTION_DAYS", strconv.Itoa(*events.RetentionDays))
	}
	if events.CleanupInterval != nil {
		setEnv(env, "EVENT_CLEANUP_INTERVAL", strconv.Itoa(*events.CleanupInterval))
	}
	if events.CaptureIPAddress != nil {
		setEnv(env, "EVENT_CAPTURE_IP_ADDRESS", strconv.FormatBool(*events.CaptureIPAddress))
	}
}

func projectSystemMicroserviceEnv(env map[string]string, sys *struct {
	Router map[string]string `yaml:"router,omitempty" json:"router,omitempty"`
	NATS   map[string]string `yaml:"nats,omitempty" json:"nats,omitempty"`
}) {
	if sys == nil {
		return
	}
	for _, slot := range controlPlaneRouterArchSlots {
		if sys.Router != nil {
			setEnvIfNonEmpty(env, fmt.Sprintf("ROUTER_IMAGE_%d", slot.slot), sys.Router[slot.arch])
		}
		if sys.NATS != nil {
			setEnvIfNonEmpty(env, fmt.Sprintf("NATS_IMAGE_%d", slot.slot), sys.NATS[slot.arch])
		}
	}
}

func projectNATSEnv(env map[string]string, nats *struct {
	Enabled *bool `yaml:"enabled,omitempty" json:"enabled,omitempty"`
}) {
	if nats == nil || nats.Enabled == nil {
		return
	}
	setEnv(env, "NATS_ENABLED", strconv.FormatBool(*nats.Enabled))
}

func projectHTTPSEnv(env map[string]string, https *models.ControlPlaneHTTPSConfig) {
	if https == nil {
		return
	}
	if path := strings.TrimSpace(https.Path); path != "" {
		setEnv(env, "SSL_PATH_CERT", models.ControlPlaneHTTPSCertFilename)
		setEnv(env, "SSL_PATH_KEY", models.ControlPlaneHTTPSKeyFilename)
		if _, err := os.Stat(filepath.Join(path, models.ControlPlaneHTTPSCAFilename)); err == nil {
			setEnv(env, "SSL_PATH_INTERMEDIATE_CERT", models.ControlPlaneHTTPSCAFilename)
		}
		return
	}
	if https.Base64 == nil {
		return
	}
	setEnvIfNonEmpty(env, "SSL_BASE64_CERT", https.Base64.Cert)
	setEnvIfNonEmpty(env, "SSL_BASE64_KEY", https.Base64.Key)
	setEnvIfNonEmpty(env, "SSL_BASE64_INTERMEDIATE_CERT", https.Base64.CA)
}

func projectVaultEnv(env map[string]string, vault *models.ControlPlaneVaultSpec) {
	if vault == nil {
		return
	}
	if vault.Enabled != nil {
		setEnv(env, "VAULT_ENABLED", strconv.FormatBool(*vault.Enabled))
	}
	setEnvIfNonEmpty(env, "VAULT_PROVIDER", vault.Provider)
	setEnvIfNonEmpty(env, "VAULT_BASE_PATH", vault.BasePath)
	if vault.Hashicorp != nil {
		setEnvIfNonEmpty(env, "VAULT_HASHICORP_ADDRESS", vault.Hashicorp.Address)
		setEnvIfNonEmpty(env, "VAULT_HASHICORP_TOKEN", vault.Hashicorp.Token)
		setEnvIfNonEmpty(env, "VAULT_HASHICORP_MOUNT", vault.Hashicorp.Mount)
	}
	if vault.AWS != nil {
		setEnvIfNonEmpty(env, "VAULT_AWS_REGION", vault.AWS.Region)
		setEnvIfNonEmpty(env, "VAULT_AWS_ACCESS_KEY_ID", vault.AWS.AccessKeyID)
		setEnvIfNonEmpty(env, "VAULT_AWS_ACCESS_KEY", vault.AWS.AccessKey)
	}
	if vault.Azure != nil {
		setEnvIfNonEmpty(env, "VAULT_AZURE_URL", vault.Azure.URL)
		setEnvIfNonEmpty(env, "VAULT_AZURE_TENANT_ID", vault.Azure.TenantID)
		setEnvIfNonEmpty(env, "VAULT_AZURE_CLIENT_ID", vault.Azure.ClientID)
		setEnvIfNonEmpty(env, "VAULT_AZURE_CLIENT_SECRET", vault.Azure.ClientSecret)
	}
	if vault.Google != nil {
		setEnvIfNonEmpty(env, "VAULT_GOOGLE_PROJECT_ID", vault.Google.ProjectID)
		setEnvIfNonEmpty(env, "VAULT_GOOGLE_CREDENTIALS", vault.Google.Credentials)
	}
}

func setEnv(env map[string]string, key, value string) {
	env[key] = value
}

func setEnvIfNonEmpty(env map[string]string, key, value string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	setEnv(env, key, strings.TrimSpace(value))
}
