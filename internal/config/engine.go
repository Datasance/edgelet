package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/datasance/edgelet/internal/constants"
)

// ChangeClass describes how a config mutation affects the daemon.
type ChangeClass int

const (
	ChangeClassHot ChangeClass = iota
	ChangeClassWarm
	ChangeClassCold
)

// DefaultDockerURLForEngine returns the platform default socket URL for an engine family.
func DefaultDockerURLForEngine(engine string) string {
	switch strings.ToLower(strings.TrimSpace(engine)) {
	case constants.EngineEdgelet:
		return constants.EdgeletEngineDockerURL()
	case constants.EnginePodman:
		return constants.PodmanDefaultDockerURL
	default:
		return "unix:///var/run/docker.sock"
	}
}

// ClassifyEngineConfigChange compares startup vs reloaded config.
func ClassifyEngineConfigChange(startupEngine, newEngine, startupURL, newURL string, _ map[string]struct{}) ChangeClass {
	startupEngine = strings.ToLower(strings.TrimSpace(startupEngine))
	newEngine = strings.ToLower(strings.TrimSpace(newEngine))

	if startupEngine != newEngine {
		return ChangeClassCold
	}

	startupURL = strings.TrimSpace(startupURL)
	newURL = strings.TrimSpace(newURL)
	if startupURL != newURL {
		if newEngine == constants.EngineDocker || newEngine == constants.EnginePodman {
			return ChangeClassWarm
		}
	}

	return ChangeClassHot
}

// ApplyEngineDefaults updates dockerUrl when containerEngine changes in-memory and YAML.
func (c *Config) ApplyEngineDefaults(engine string) error {
	engine = strings.ToLower(strings.TrimSpace(engine))
	defaultURL := DefaultDockerURLForEngine(engine)

	c.mu.Lock()
	c.ContainerEngine = engine
	c.DockerURL = defaultURL
	yaml := c.yamlConfig
	profileName := c.currentProfile.FullValue()
	c.mu.Unlock()

	if yaml == nil {
		return nil
	}
	profile := yaml.GetProfile(profileName)
	if profile == nil {
		return nil
	}
	profile.SetProperty("containerEngine", engine)
	profile.SetProperty("dockerUrl", defaultURL)
	return nil
}

// SnapshotYAML returns a copy of the on-disk YAML bytes for revert on failed warm reload.
func SnapshotYAML(path string) ([]byte, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("config path is empty")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config snapshot: %w", err)
	}
	out := make([]byte, len(data))
	copy(out, data)
	return out, nil
}

// RevertDockerURL restores dockerUrl in memory and YAML after a failed warm reload.
func (c *Config) RevertDockerURL(dockerURL string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.DockerURL = dockerURL
	if err := c.setYamlProperty("dockerUrl", dockerURL); err != nil {
		return err
	}
	return c.saveConfigUpdatesLocked()
}

// PatchKeysFromMap converts a SetConfig map to short-code keys for classification.
func PatchKeysFromMap(configMap map[string]interface{}) map[string]struct{} {
	keys := make(map[string]struct{}, len(configMap))
	for k := range configMap {
		keys[k] = struct{}{}
	}
	return keys
}
