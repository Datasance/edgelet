package config

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	"github.com/eclipse-iofog/edgelet/internal/utils"
	"gopkg.in/yaml.v3"
)

//go:embed default_config.yaml
var embeddedDefaultConfigYAML []byte

const sampleConfigSharePath = "/usr/share/edgelet/edgelet-config.yaml.sample"

// InitConfig writes the default config template when the target path is missing.
// If the file already exists, it is left unchanged and created is false.
func InitConfig(configPath string) (created bool, err error) {
	if configPath == "" {
		configPath = utils.ConfigYAMLPath
	}
	cleanPath := filepath.Clean(configPath)

	if _, err := os.Stat(cleanPath); err == nil {
		return false, nil
	} else if !os.IsNotExist(err) {
		return false, fmt.Errorf("stat config %s: %w", cleanPath, err)
	}

	data, err := defaultConfigBytes()
	if err != nil {
		return false, err
	}

	dir := filepath.Dir(cleanPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return false, fmt.Errorf("create config directory %s: %w", dir, err)
	}

	tmpFile, err := os.CreateTemp(dir, ".config-init-*.tmp")
	if err != nil {
		return false, fmt.Errorf("create temp config file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer func() { _ = os.Remove(tmpPath) }()

	if err := tmpFile.Chmod(0640); err != nil {
		_ = tmpFile.Close()
		return false, fmt.Errorf("chmod temp config file: %w", err)
	}
	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		return false, fmt.Errorf("write temp config file: %w", err)
	}
	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		return false, fmt.Errorf("sync temp config file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return false, fmt.Errorf("close temp config file: %w", err)
	}

	if err := os.Rename(tmpPath, cleanPath); err != nil {
		return false, fmt.Errorf("install config at %s: %w", cleanPath, err)
	}
	_ = syncDirectory(dir)

	return true, nil
}

func defaultConfigBytes() ([]byte, error) {
	if data, err := os.ReadFile(sampleConfigSharePath); err == nil {
		return data, nil
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read %s: %w", sampleConfigSharePath, err)
	}
	if len(embeddedDefaultConfigYAML) > 0 {
		return embeddedDefaultConfigYAML, nil
	}
	yamlConfig := createDefaultYamlConfigForLoader()
	data, err := yaml.Marshal(yamlConfig)
	if err != nil {
		return nil, fmt.Errorf("marshal default config: %w", err)
	}
	return data, nil
}
