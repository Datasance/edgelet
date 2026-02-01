package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/eclipse-iofog/agent-go/internal/models"
	"github.com/eclipse-iofog/agent-go/internal/utils"
	"github.com/eclipse-iofog/agent-go/internal/utils/logging"
	"gopkg.in/yaml.v3"
)

// LoadConfig loads configuration from YAML file
func LoadConfig(configPath string) error {
	cfg := GetInstance()

	// Try to load from primary config file
	yamlConfig, err := loadYAMLFile(configPath)
	if err != nil {
		// Try backup config file
		backupPath := utils.BackupConfigYAMLPath
		yamlConfig, err = loadYAMLFile(backupPath)
		if err != nil {
			// If both fail, create a default config
			logging.LogWarn("Config Loader", "Config file not found, creating default config")
			yamlConfig = createDefaultYamlConfigForLoader()
			// Try to save the default config (create directory if needed)
			if err := os.MkdirAll(utils.ConfigDir, 0755); err == nil {
				if saveErr := SaveConfig(configPath); saveErr != nil {
					logging.LogWarn("Config Loader", fmt.Sprintf("Failed to save default config: %v", saveErr))
				}
			}
		}
	}

	// Validate and set current profile
	currentProfileStr := yamlConfig.CurrentProfile
	if currentProfileStr == "" {
		currentProfileStr = utils.ConfigSwitcherStateDefault.FullValue()
		yamlConfig.CurrentProfile = currentProfileStr
	}

	currentProfile, err := utils.ParseConfigSwitcherState(currentProfileStr)
	if err != nil {
		// Use default profile
		currentProfile = utils.ConfigSwitcherStateDefault
		yamlConfig.CurrentProfile = currentProfile.FullValue()
	}

	// Ensure current profile exists
	if yamlConfig.GetProfile(currentProfile.FullValue()) == nil {
		currentProfile = utils.ConfigSwitcherStateDefault
		yamlConfig.CurrentProfile = currentProfile.FullValue()
	}

	// Set the config
	cfg.SetYamlConfig(yamlConfig)
	cfg.SetCurrentProfile(currentProfile)
	cfg.SetConfigPath(configPath)

	// Load all configuration values from current profile
	loadConfigValues(cfg)

	return nil
}

// loadYAMLFile loads a YAML file
func loadYAMLFile(path string) (*models.YamlConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var yamlConfig models.YamlConfig
	if err := yaml.Unmarshal(data, &yamlConfig); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	if yamlConfig.Profiles == nil {
		yamlConfig.Profiles = make(map[string]*models.ProfileConfig)
	}

	return &yamlConfig, nil
}

// loadConfigValues loads all configuration values from the current profile
func loadConfigValues(cfg *Config) {
	profile := cfg.GetYamlConfig().GetProfile(cfg.GetCurrentProfile().FullValue())
	if profile == nil {
		return
	}

	// Helper to get property with default
	getProp := func(key, defaultValue string) string {
		val := profile.GetProperty(key)
		if val == "" {
			return defaultValue
		}
		return val
	}

	// Load all config values
	// Note: This is a simplified version. Full implementation would parse all values
	// and handle type conversions properly
	cfg.IOFogUUID = getProp("iofogUuid", "")
	cfg.PrivateKey = getProp("privateKey", "")
	cfg.ControllerURL = getProp("controllerUrl", "https://fogcontroller1.iofog.org:54421/api/v2/")
	cfg.ControllerCert = getProp("controllerCert", "/etc/iofog-agent/cert.crt")
	cfg.NetworkInterface = getProp("networkInterface", "dynamic")
	cfg.DockerURL = getProp("dockerUrl", "unix:///var/run/docker.sock")
	cfg.DiskDirectory = getProp("diskDirectory", "/var/lib/iofog-agent/")
	cfg.LogDiskDirectory = getProp("logDiskDirectory", "/var/log/iofog-agent/")
	cfg.LogLevel = strings.ToUpper(getProp("logLevel", "INFO"))
	cfg.GPSDevice = getProp("gpsDevice", "/dev/ttyUSB0")
	cfg.GPSMode = getProp("gpsMode", "auto")
	cfg.GPSCoordinates = getProp("gpsCoordinates", "")
	cfg.Arch = getProp("arch", "auto")
	cfg.RouterHost = getProp("routerHost", "")
	cfg.Namespace = getProp("namespace", "default")
	cfg.TimeZone = getProp("timeZone", "")
	cfg.CACert = getProp("caCert", "")
	cfg.TLSCert = getProp("tlsCert", "")
	cfg.TLSKey = getProp("tlsKey", "")
	// HWSignature removed - now stored in separate file: /etc/iofog-agent/agent-{uuid}.jwt
	// This prevents triggering SIGHUP/reload when signature is updated
	// cfg.HWSignature = getProp("hwSignature", "")

	// Parse numeric values (simplified - should handle errors properly)
	parseFloat := func(key, defaultValue string) float64 {
		val := getProp(key, defaultValue)
		var result float64
		fmt.Sscanf(val, "%f", &result)
		return result
	}

	parseInt := func(key, defaultValue string) int {
		val := getProp(key, defaultValue)
		var result int
		fmt.Sscanf(val, "%d", &result)
		return result
	}

	parseInt64 := func(key, defaultValue string) int64 {
		val := getProp(key, defaultValue)
		var result int64
		fmt.Sscanf(val, "%d", &result)
		return result
	}

	cfg.DiskLimit = parseFloat("diskLimit", "10")
	cfg.MemoryLimit = parseFloat("memoryLimit", "4096")
	cfg.CPULimit = parseFloat("cpuLimit", "80")
	cfg.LogDiskLimit = parseFloat("logLimit", "10")
	cfg.LogFileCount = parseInt("logFileCount", "10")
	cfg.StatusFrequency = parseInt("statusFrequency", "10")
	cfg.ChangeFrequency = parseInt("changeFrequency", "20")
	cfg.DeviceScanFrequency = parseInt("deviceScanFrequency", "60")
	cfg.PostDiagnosticsFreq = parseInt("postDiagnosticsFreq", "10")
	cfg.WatchdogEnabled = getProp("watchdogEnabled", "off") != "off"
	cfg.EdgeGuardFrequency = parseInt64("edgeGuardFrequency", "0")
	cfg.GPSScanFrequency = parseInt64("gpsScanFrequency", "60")
	cfg.SecureMode = getProp("secureMode", "off") != "off"
	cfg.RouterPort = parseInt("routerPort", "0")
	cfg.DockerPruningFrequency = parseInt64("dockerPruningFrequency", "0")
	cfg.AvailableDiskThreshold = parseInt64("availableDiskThreshold", "20")
	cfg.ReadyToUpgradeScanFrequency = parseInt("readyToUpgradeScanFrequency", "24")
	cfg.DevMode = getProp("devMode", "off") != "off"

	// Update automatic config params based on architecture
	updateAutomaticConfigParams(cfg)
}

// createDefaultYamlConfigForLoader creates a default YAML config for the loader
func createDefaultYamlConfigForLoader() *models.YamlConfig {
	yamlConfig := models.NewYamlConfig()
	yamlConfig.CurrentProfile = utils.ConfigSwitcherStateDefault.FullValue()

	// Create default profile with default values
	defaultProfile := models.NewProfileConfig()
	defaultProfile.SetProperty("controllerUrl", "http://localhost:54421/api/v3/")
	defaultProfile.SetProperty("controllerCert", "/etc/iofog-agent/cert.crt")
	defaultProfile.SetProperty("networkInterface", "dynamic")
	defaultProfile.SetProperty("dockerUrl", "unix:///var/run/docker.sock")
	defaultProfile.SetProperty("diskDirectory", "/var/lib/iofog-agent/")
	defaultProfile.SetProperty("diskLimit", "10")
	defaultProfile.SetProperty("memoryLimit", "4096")
	defaultProfile.SetProperty("cpuLimit", "80")
	defaultProfile.SetProperty("logDirectory", "/var/log/iofog-agent/")
	defaultProfile.SetProperty("logLimit", "10")
	defaultProfile.SetProperty("logFileCount", "10")
	defaultProfile.SetProperty("logLevel", "INFO")
	defaultProfile.SetProperty("statusFrequency", "10")
	defaultProfile.SetProperty("changeFrequency", "20")
	defaultProfile.SetProperty("deviceScanFrequency", "60")
	defaultProfile.SetProperty("postDiagnosticsFreq", "10")
	defaultProfile.SetProperty("watchdogEnabled", "off")
	defaultProfile.SetProperty("edgeGuardFrequency", "0")
	defaultProfile.SetProperty("gpsDevice", "/dev/ttyUSB0")
	defaultProfile.SetProperty("gpsScanFrequency", "60")
	defaultProfile.SetProperty("gpsMode", "auto")
	defaultProfile.SetProperty("gpsCoordinates", "0,0")
	defaultProfile.SetProperty("arch", "auto")
	defaultProfile.SetProperty("secureMode", "off")
	defaultProfile.SetProperty("dockerPruningFrequency", "0")
	defaultProfile.SetProperty("availableDiskThreshold", "20")
	defaultProfile.SetProperty("readyToUpgradeScanFrequency", "24")
	defaultProfile.SetProperty("devMode", "off")
	defaultProfile.SetProperty("timeZone", "")
	defaultProfile.SetProperty("namespace", "default")

	yamlConfig.Profiles[utils.ConfigSwitcherStateDefault.FullValue()] = defaultProfile

	return yamlConfig
}

// updateAutomaticConfigParams updates automatic configuration parameters based on architecture
func updateAutomaticConfigParams(cfg *Config) {
	// For now, use same values for all architectures
	// This can be customized based on arch type
	cfg.StatusReportFreqSeconds = 5
	cfg.PingControllerFreqSeconds = 30
	cfg.SpeedCalculationFreqMinutes = 1
	cfg.MonitorContainersStatusFreqSeconds = 5
	cfg.MonitorRegistriesStatusFreqSeconds = 60
	cfg.GetUsageDataFreqSeconds = 5
	cfg.DockerAPIVersion = "1.44"
	cfg.SetSystemTimeFreqSeconds = 5
	cfg.MonitorSSHTunnelStatusFreqSeconds = 30
}

// SaveConfig saves the configuration to YAML file
func SaveConfig(configPath string) error {
	cfg := GetInstance()
	yamlConfig := cfg.GetYamlConfig()
	if yamlConfig == nil {
		return ErrConfigNotLoaded
	}

	return SaveConfigWithYaml(configPath, yamlConfig)
}

// SaveConfigWithYaml saves the provided YAML config to file (avoids lock acquisition)
func SaveConfigWithYaml(configPath string, yamlConfig *models.YamlConfig) error {
	if yamlConfig == nil {
		return ErrConfigNotLoaded
	}

	data, err := yaml.Marshal(yamlConfig)
	if err != nil {
		return fmt.Errorf("failed to marshal YAML: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}
