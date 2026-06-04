package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/datasance/edgelet/internal/models"
	"github.com/datasance/edgelet/internal/utils"
)

func TestLoadConfigMissingFilesCreatesDefaultConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	backupPath := filepath.Join(tmpDir, "config-bck.yaml")

	prevDir := utils.ConfigDir
	prevBackup := utils.BackupConfigYAMLPath
	utils.ConfigDir = tmpDir + string(os.PathSeparator)
	utils.BackupConfigYAMLPath = backupPath
	defer func() {
		utils.ConfigDir = prevDir
		utils.BackupConfigYAMLPath = prevBackup
	}()

	if err := LoadConfig(configPath); err != nil {
		t.Fatalf("expected default config bootstrap to succeed, got error: %v", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("expected default config file to be written: %v", err)
	}
	if !strings.Contains(string(data), "currentProfile") {
		t.Fatalf("expected saved default config to include currentProfile, got: %s", string(data))
	}
}

func TestLoadConfigParseErrorDoesNotGenerateDefaults(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	backupPath := filepath.Join(tmpDir, "config-bck.yaml")
	invalidYAML := "currentProfile: [broken"

	if err := os.WriteFile(configPath, []byte(invalidYAML), 0600); err != nil {
		t.Fatalf("failed to write primary config: %v", err)
	}
	if err := os.WriteFile(backupPath, []byte(invalidYAML), 0600); err != nil {
		t.Fatalf("failed to write backup config: %v", err)
	}

	prevBackup := utils.BackupConfigYAMLPath
	utils.BackupConfigYAMLPath = backupPath
	defer func() { utils.BackupConfigYAMLPath = prevBackup }()

	err := LoadConfig(configPath)
	if err == nil {
		t.Fatal("expected parse failure to be returned, got nil")
	}

	data, readErr := os.ReadFile(configPath)
	if readErr != nil {
		t.Fatalf("failed to read primary config after parse failure: %v", readErr)
	}
	if string(data) != invalidYAML {
		t.Fatalf("primary config should remain unchanged after parse failure; got: %s", string(data))
	}
}

func TestSaveConfigWithYamlAtomicWrite(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	yamlCfg := models.NewYamlConfig()
	yamlCfg.CurrentProfile = "default"
	profile := models.NewProfileConfig()
	profile.SetProperty("logLevel", "DEBUG")
	yamlCfg.Profiles["default"] = profile

	if err := SaveConfigWithYaml(configPath, yamlCfg); err != nil {
		t.Fatalf("SaveConfigWithYaml failed: %v", err)
	}

	loaded, err := loadYAMLFile(configPath)
	if err != nil {
		t.Fatalf("saved file should be valid yaml, got error: %v", err)
	}
	if loaded.GetProfile("default").GetProperty("logLevel") != "DEBUG" {
		t.Fatalf("expected persisted logLevel DEBUG, got %q", loaded.GetProfile("default").GetProperty("logLevel"))
	}

	tmpMatches, err := filepath.Glob(filepath.Join(tmpDir, ".config-*.tmp"))
	if err != nil {
		t.Fatalf("failed to glob temp files: %v", err)
	}
	if len(tmpMatches) != 0 {
		t.Fatalf("expected no leftover temp files, found %d: %v", len(tmpMatches), tmpMatches)
	}
}

func TestLoadConfigFailureKeepsLastGoodRuntimeValues(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	backupPath := filepath.Join(tmpDir, "config-bck.yaml")

	validYAML := `currentProfile: default
profiles:
  default:
    pruningFrequency: "9"
`
	if err := os.WriteFile(configPath, []byte(validYAML), 0600); err != nil {
		t.Fatalf("failed to write valid primary config: %v", err)
	}

	prevBackup := utils.BackupConfigYAMLPath
	utils.BackupConfigYAMLPath = backupPath
	defer func() { utils.BackupConfigYAMLPath = prevBackup }()

	if err := LoadConfig(configPath); err != nil {
		t.Fatalf("expected initial valid load to succeed: %v", err)
	}
	if got := GetInstance().PruningFrequency; got != 9 {
		t.Fatalf("expected initial pruningFrequency=9, got %d", got)
	}

	invalidYAML := "currentProfile: [invalid"
	if err := os.WriteFile(configPath, []byte(invalidYAML), 0600); err != nil {
		t.Fatalf("failed to overwrite primary config with invalid yaml: %v", err)
	}
	if err := os.WriteFile(backupPath, []byte(invalidYAML), 0600); err != nil {
		t.Fatalf("failed to write invalid backup yaml: %v", err)
	}

	if err := LoadConfig(configPath); err == nil {
		t.Fatal("expected malformed reload to fail")
	}
	if got := GetInstance().PruningFrequency; got != 9 {
		t.Fatalf("expected runtime to keep last-good pruningFrequency=9 after failed reload, got %d", got)
	}
}
