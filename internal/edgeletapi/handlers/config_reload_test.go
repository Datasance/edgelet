package handlers

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/eclipse-iofog/edgelet/internal/buildmeta"
	"github.com/eclipse-iofog/edgelet/internal/config"
	"github.com/eclipse-iofog/edgelet/internal/constants"
	"github.com/eclipse-iofog/edgelet/internal/models"
	"github.com/eclipse-iofog/edgelet/internal/utils"
	"github.com/eclipse-iofog/edgelet/internal/utils/logging"
	"github.com/sirupsen/logrus"
)

func TestHandleConfigPatch_LogLevelTriggersFullReload(t *testing.T) {
	cfg := setupConfigForGPSTests(t)
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	engine := constants.EngineDocker
	engineURL := "unix:///var/run/docker.sock"
	if buildmeta.HasEmbeddedEngine() {
		engine = constants.EngineEdgelet
		engineURL = constants.EdgeletEngineSocketURL()
	}

	yaml := `currentProfile: default
profiles:
  default:
    logLevel: "INFO"
    diskLimit: "10"
    memoryLimit: "4096"
    cpuLimit: "80"
    containerEngine: "` + engine + `"
    containerEngineUrl: "` + engineURL + `"
`
	if err := os.WriteFile(configPath, []byte(yaml), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg.SetConfigPath(configPath)

	yamlCfg := models.NewYamlConfig()
	yamlCfg.CurrentProfile = utils.ConfigSwitcherStateDefault.FullValue()
	profile := models.NewProfileConfig()
	profile.SetProperty("logLevel", "INFO")
	profile.SetProperty("diskLimit", "10")
	profile.SetProperty("memoryLimit", "4096")
	profile.SetProperty("cpuLimit", "80")
	profile.SetProperty("containerEngine", engine)
	profile.SetProperty("containerEngineUrl", engineURL)
	yamlCfg.Profiles[utils.ConfigSwitcherStateDefault.FullValue()] = profile
	cfg.SetYamlConfig(yamlCfg)
	cfg.LogLevel = "INFO"

	logDir := filepath.Join(tmpDir, "logs")
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		t.Fatalf("mkdir logs: %v", err)
	}
	if err := logging.SetupLogger(logDir, 10, 10, "INFO"); err != nil {
		t.Fatalf("setup logger: %v", err)
	}

	notifyCalled := false
	cfg.SetReloadCallback(func() error {
		return config.FullReload(config.ReloadHooks{
			ConfigPath: configPath,
			NotifyModules: func() error {
				notifyCalled = true
				return nil
			},
		})
	})

	handler := NewEdgeletAPIHandler()
	req := httptest.NewRequest(http.MethodPatch, "/v1/system/config", bytes.NewBufferString(`{"set":{"logLevel":"DEBUG"}}`))
	rec := httptest.NewRecorder()
	handler.HandleConfig(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !notifyCalled {
		t.Fatal("expected full reload to notify modules")
	}
	if logging.GetInstance().GetLevel() != logrus.DebugLevel {
		t.Fatalf("expected log level DEBUG after PATCH reload, got %v", logging.GetInstance().GetLevel())
	}
}

func TestHandleSystemReload_Returns500OnReloadFailure(t *testing.T) {
	cfg := setupConfigForGPSTests(t)
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(`currentProfile: default
profiles:
  default:
    logLevel: "NOT_A_LEVEL"
    diskLimit: "10"
    memoryLimit: "4096"
    cpuLimit: "80"
`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg.SetReloadCallback(func() error {
		return config.FullReload(config.ReloadHooks{ConfigPath: configPath})
	})

	handler := NewEdgeletAPIHandler()
	req := httptest.NewRequest(http.MethodPost, "/v1/system/reload", nil)
	rec := httptest.NewRecorder()
	handler.HandleSystemReload(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d body=%s", rec.Code, rec.Body.String())
	}
}
