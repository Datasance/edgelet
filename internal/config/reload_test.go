package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/eclipse-iofog/edgelet/internal/buildmeta"
	"github.com/eclipse-iofog/edgelet/internal/constants"
	"github.com/eclipse-iofog/edgelet/internal/utils/logging"
	"github.com/sirupsen/logrus"
)

func TestFullReload_UpdatesLogLevelWithoutRestart(t *testing.T) {
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
    logLevel: "DEBUG"
    diskLimit: "10"
    memoryLimit: "4096"
    cpuLimit: "80"
    containerEngine: "` + engine + `"
    containerEngineUrl: "` + engineURL + `"
`
	if err := os.WriteFile(configPath, []byte(yaml), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	logDir := filepath.Join(tmpDir, "logs")
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		t.Fatalf("mkdir logs: %v", err)
	}
	if err := logging.SetupLogger(logDir, 10, 10, "INFO"); err != nil {
		t.Fatalf("setup logger: %v", err)
	}
	if logging.GetInstance().GetLevel() != logrus.InfoLevel {
		t.Fatalf("expected initial log level INFO, got %v", logging.GetInstance().GetLevel())
	}

	notifyCalled := false
	if err := FullReload(ReloadHooks{
		ConfigPath: configPath,
		NotifyModules: func() error {
			notifyCalled = true
			return nil
		},
	}); err != nil {
		t.Fatalf("FullReload failed: %v", err)
	}
	if !notifyCalled {
		t.Fatal("expected NotifyModules to be called")
	}
	if logging.GetInstance().GetLevel() != logrus.DebugLevel {
		t.Fatalf("expected log level DEBUG after reload, got %v", logging.GetInstance().GetLevel())
	}
	if !IsLastReloadSuccessful() {
		t.Fatal("expected last reload to be marked successful")
	}
}

func TestFullReload_InvalidConfigReturnsError(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	yaml := `currentProfile: default
profiles:
  default:
    logLevel: "NOT_A_LEVEL"
    diskLimit: "10"
    memoryLimit: "4096"
    cpuLimit: "80"
`
	if err := os.WriteFile(configPath, []byte(yaml), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if err := FullReload(ReloadHooks{ConfigPath: configPath}); err == nil {
		t.Fatal("expected FullReload to fail for invalid config")
	}
	if IsLastReloadSuccessful() {
		t.Fatal("expected last reload to be marked unsuccessful")
	}
}
