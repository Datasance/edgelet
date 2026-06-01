package config

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/datasance/edgelet/internal/models"
	"github.com/datasance/edgelet/internal/utils"
	"github.com/datasance/edgelet/internal/utils/logging"
)

const (
	setConfigModuleName = "Config SetConfig"
)

// ConfigParamMap maps command line parameter names to config fields
var ConfigParamMap = map[string]string{
	"d":    "diskLimit",
	"dl":   "diskDirectory",
	"m":    "memoryLimit",
	"p":    "cpuLimit",
	"a":    "controllerURL",
	"ac":   "controllerCert",
	"c":    "dockerURL",
	"ce":   "containerEngine",
	"n":    "networkInterface",
	"l":    "logDiskLimit",
	"ld":   "logDiskDirectory",
	"lc":   "logFileCount",
	"ll":   "logLevel",
	"sf":   "statusFrequency",
	"cf":   "changeFrequency",
	"sd":   "deviceScanFrequency",
	"idc":  "watchdogEnabled",
	"egf":  "edgeGuardFrequency",
	"gps":  "gpsMode",
	"gpsc": "gpsCoordinates",
	"gpsd": "gpsDevice",
	"gpsf": "gpsScanFrequency",
	"ft":   "arch",
	"sec":  "secureMode",
	"pf":   "dockerPruningFrequency",
	"dt":   "availableDiskThreshold",
	"uf":   "upgradeScanFrequency",
	"dev":  "devMode",
	"tz":   "timeZone",
}

// SetConfig sets configuration values from a map of command line parameters
// Returns a map of errors keyed by parameter name
func (c *Config) SetConfig(configMap map[string]interface{}) map[string]string {
	c.mu.Lock()
	defer c.mu.Unlock()

	errorMap := make(map[string]string)

	for option, value := range configMap {
		// Get config field name
		fieldName, exists := ConfigParamMap[option]
		if !exists {
			errorMap[option] = "Invalid parameter"
			continue
		}

		// Convert value to string
		valueStr := fmt.Sprintf("%v", value)

		// Remove leading "+" if present
		valueStr = strings.TrimPrefix(valueStr, "+")

		// Validate
		if strings.TrimSpace(valueStr) == "" && option != "ac" {
			errorMap[option] = "Command or value is invalid"
			continue
		}

		// Set the config value based on field name
		err := c.setConfigField(fieldName, valueStr, option)
		if err != nil {
			errorMap[option] = err.Error()
			logging.LogError(setConfigModuleName, fmt.Sprintf("Error setting %s: %v", option, err), nil)
		}
	}

	// Save config updates (while holding lock)
	if len(errorMap) == 0 {
		// Save config (don't acquire lock - we already hold it)
		if err := c.saveConfigUpdatesLocked(); err != nil {
			logging.LogError(setConfigModuleName, "Error saving config updates", err)
		}
		// Note: We don't update logger here anymore because saveConfigUpdatesLocked writes to file,
		// which triggers the file watcher in main.go. The watcher callback will reload config
		// and update the logger. This prevents a double-update race condition.
	}

	// Notify modules asynchronously (don't block)
	// We rely on the file watcher to detect the config file change and trigger reload via SIGHUP
	// This prevents double reloads (one explicit, one from watcher)
	/*
		if len(errorMap) == 0 {
			go c.notifyModulesOfConfigChangeAsync()
		}
	*/

	return errorMap
}

// NotifyModulesOfConfigChange notifies modules when configuration changes
// This should be called after SetConfig to update modules like FieldAgent
func (c *Config) NotifyModulesOfConfigChange() error {
	// This will be called from supervisor or CLI to avoid import cycles
	// The actual implementation will be in supervisor
	return nil
}

// SwitchProfile sets active configuration profile and persists currentProfile.
func (c *Config) SwitchProfile(profile utils.ConfigSwitcherState) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.currentProfile = profile
	if err := c.saveConfigUpdatesLocked(); err != nil {
		return fmt.Errorf("failed to persist switched profile: %w", err)
	}
	return nil
}

// setConfigField sets a specific config field value
func (c *Config) setConfigField(fieldName, value, _ string) error {
	switch fieldName {
	case "diskLimit":
		val, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return fmt.Errorf("invalid disk limit: %w", err)
		}
		c.DiskLimit = val
		if err := c.setYamlProperty("diskLimit", value); err != nil {
			logging.LogWarn(setConfigModuleName, fmt.Sprintf("Failed to persist config property: %v", err))
		}

	case "diskDirectory":
		c.DiskDirectory = value
		if err := c.setYamlProperty("diskDirectory", value); err != nil {
			logging.LogWarn(setConfigModuleName, fmt.Sprintf("Failed to persist config property: %v", err))
		}

	case "memoryLimit":
		val, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return fmt.Errorf("invalid memory limit: %w", err)
		}
		c.MemoryLimit = val
		if err := c.setYamlProperty("memoryLimit", value); err != nil {
			logging.LogWarn(setConfigModuleName, fmt.Sprintf("Failed to persist config property: %v", err))
		}

	case "cpuLimit":
		val, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return fmt.Errorf("invalid CPU limit: %w", err)
		}
		c.CPULimit = val
		if err := c.setYamlProperty("cpuLimit", value); err != nil {
			logging.LogWarn(setConfigModuleName, fmt.Sprintf("Failed to persist config property: %v", err))
		}

	case "controllerURL":
		c.ControllerURL = value
		if err := c.setYamlProperty("controllerUrl", value); err != nil {
			logging.LogWarn(setConfigModuleName, fmt.Sprintf("Failed to persist config property: %v", err))
		}

	case "controllerCert":
		c.ControllerCert = value
		if err := c.setYamlProperty("controllerCert", value); err != nil {
			logging.LogWarn(setConfigModuleName, fmt.Sprintf("Failed to persist config property: %v", err))
		}

	case "dockerURL":
		c.DockerURL = value
		if err := c.setYamlProperty("dockerUrl", value); err != nil {
			logging.LogWarn(setConfigModuleName, fmt.Sprintf("Failed to persist config property: %v", err))
		}

	case "containerEngine":
		switch value {
		case "docker", "podman", "edgelet":
			c.ContainerEngine = value
			if err := c.setYamlProperty("containerEngine", value); err != nil {
				logging.LogWarn(setConfigModuleName, fmt.Sprintf("Failed to persist config property: %v", err))
			}
		default:
			return fmt.Errorf("invalid container engine %q: must be one of docker, podman, iofog", value)
		}

	case "networkInterface":
		c.NetworkInterface = value
		if err := c.setYamlProperty("networkInterface", value); err != nil {
			logging.LogWarn(setConfigModuleName, fmt.Sprintf("Failed to persist config property: %v", err))
		}

	case "logDiskLimit":
		val, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return fmt.Errorf("invalid log disk limit: %w", err)
		}
		c.LogDiskLimit = val
		if err := c.setYamlProperty("logDiskLimit", value); err != nil {
			logging.LogWarn(setConfigModuleName, fmt.Sprintf("Failed to persist config property: %v", err))
		}

	case "logDiskDirectory":
		c.LogDiskDirectory = value
		if err := c.setYamlProperty("logDiskDirectory", value); err != nil {
			logging.LogWarn(setConfigModuleName, fmt.Sprintf("Failed to persist config property: %v", err))
		}

	case "logFileCount":
		val, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid log file count: %w", err)
		}
		c.LogFileCount = val
		if err := c.setYamlProperty("logFileCount", value); err != nil {
			logging.LogWarn(setConfigModuleName, fmt.Sprintf("Failed to persist config property: %v", err))
		}

	case "logLevel":
		c.LogLevel = value
		if err := c.setYamlProperty("logLevel", value); err != nil {
			logging.LogWarn(setConfigModuleName, fmt.Sprintf("Failed to persist config property: %v", err))
		}

	case "statusFrequency":
		val, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid status frequency: %w", err)
		}
		c.StatusFrequency = val
		if err := c.setYamlProperty("statusFrequency", value); err != nil {
			logging.LogWarn(setConfigModuleName, fmt.Sprintf("Failed to persist config property: %v", err))
		}

	case "changeFrequency":
		val, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid change frequency: %w", err)
		}
		c.ChangeFrequency = val
		if err := c.setYamlProperty("changeFrequency", value); err != nil {
			logging.LogWarn(setConfigModuleName, fmt.Sprintf("Failed to persist config property: %v", err))
		}

	case "deviceScanFrequency":
		val, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid device scan frequency: %w", err)
		}
		c.DeviceScanFrequency = val
		if err := c.setYamlProperty("deviceScanFrequency", value); err != nil {
			logging.LogWarn(setConfigModuleName, fmt.Sprintf("Failed to persist config property: %v", err))
		}

	case "watchdogEnabled":
		normalized := strings.ToLower(strings.TrimSpace(value))
		switch normalized {
		case "off", "false", "0", "no":
			c.WatchdogEnabled = false
			normalized = "off"
		default:
			c.WatchdogEnabled = true
			normalized = "on"
		}
		if err := c.setYamlProperty("watchdogEnabled", normalized); err != nil {
			logging.LogWarn(setConfigModuleName, fmt.Sprintf("Failed to persist config property: %v", err))
		}

	case "edgeGuardFrequency":
		val, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid edge guard frequency: %w", err)
		}
		if c.IOFogUUID == "" && val > 0 {
			logging.LogWarn(setConfigModuleName, "edgeGuardFrequency cannot be enabled while agent is not provisioned; forcing value to 0")
			val = 0
		}
		c.EdgeGuardFrequency = val
		if err := c.setYamlProperty("edgeGuardFrequency", fmt.Sprintf("%d", val)); err != nil {
			logging.LogWarn(setConfigModuleName, fmt.Sprintf("Failed to persist config property: %v", err))
		}

	case "gpsMode":
		normalized := strings.ToLower(strings.TrimSpace(value))
		switch normalized {
		case "auto", "dynamic", "manual", "off":
		default:
			return fmt.Errorf("invalid GPS mode %q: must be one of auto, dynamic, manual, off", value)
		}
		c.GPSMode = normalized
		if err := c.setYamlProperty("gpsMode", normalized); err != nil {
			logging.LogWarn(setConfigModuleName, fmt.Sprintf("Failed to persist config property: %v", err))
		}

	case "gpsCoordinates":
		normalized, err := normalizeGPSCoordinates(value)
		if err != nil {
			return err
		}
		c.GPSCoordinates = normalized
		if err := c.setYamlProperty("gpsCoordinates", normalized); err != nil {
			logging.LogWarn(setConfigModuleName, fmt.Sprintf("Failed to persist config property: %v", err))
		}

	case "gpsDevice":
		c.GPSDevice = value
		if err := c.setYamlProperty("gpsDevice", value); err != nil {
			logging.LogWarn(setConfigModuleName, fmt.Sprintf("Failed to persist config property: %v", err))
		}

	case "gpsScanFrequency":
		val, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid GPS scan frequency: %w", err)
		}
		c.GPSScanFrequency = val
		if err := c.setYamlProperty("gpsScanFrequency", value); err != nil {
			logging.LogWarn(setConfigModuleName, fmt.Sprintf("Failed to persist config property: %v", err))
		}

	case "arch":
		normalized := strings.ToLower(strings.TrimSpace(value))
		if err := ValidateArch(normalized); err != nil {
			return err
		}
		c.Arch = normalized
		if err := c.setYamlProperty("arch", normalized); err != nil {
			logging.LogWarn(setConfigModuleName, fmt.Sprintf("Failed to persist config property: %v", err))
		}

	case "secureMode":
		c.SecureMode = strings.ToLower(value) != "off"
		if err := c.setYamlProperty("secureMode", value); err != nil {
			logging.LogWarn(setConfigModuleName, fmt.Sprintf("Failed to persist config property: %v", err))
		}

	case "dockerPruningFrequency":
		val, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid docker pruning frequency: %w", err)
		}
		c.DockerPruningFrequency = val
		if err := c.setYamlProperty("dockerPruningFrequency", value); err != nil {
			logging.LogWarn(setConfigModuleName, fmt.Sprintf("Failed to persist config property: %v", err))
		}

	case "availableDiskThreshold":
		val, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid available disk threshold: %w", err)
		}
		c.AvailableDiskThreshold = val
		if err := c.setYamlProperty("availableDiskThreshold", value); err != nil {
			logging.LogWarn(setConfigModuleName, fmt.Sprintf("Failed to persist config property: %v", err))
		}

	case "upgradeScanFrequency":
		val, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid ready to upgrade scan frequency: %w", err)
		}
		c.UpgradeScanFrequency = val
		if err := c.setYamlProperty("upgradeScanFrequency", value); err != nil {
			logging.LogWarn(setConfigModuleName, fmt.Sprintf("Failed to persist config property: %v", err))
		}

	case "devMode":
		c.DevMode = strings.ToLower(value) != "off"
		if err := c.setYamlProperty("devMode", value); err != nil {
			logging.LogWarn(setConfigModuleName, fmt.Sprintf("Failed to persist config property: %v", err))
		}

	case "timeZone":
		c.TimeZone = value
		if err := c.setYamlProperty("timeZone", value); err != nil {
			logging.LogWarn(setConfigModuleName, fmt.Sprintf("Failed to persist config property: %v", err))
		}

	default:
		return fmt.Errorf("unknown config field: %s", fieldName)
	}

	return nil
}

// setYamlProperty sets a property in the YAML config
func (c *Config) setYamlProperty(key, value string) error {
	if c.yamlConfig == nil {
		return nil // YAML config not loaded, skip
	}

	profile := c.yamlConfig.GetProfile(c.currentProfile.FullValue())
	if profile == nil {
		return nil // Profile not found, skip
	}

	profile.SetProperty(key, value)
	return nil
}

// saveConfigUpdates saves configuration updates to disk
// saveConfigUpdatesLocked saves configuration updates to disk
// Must be called while holding the config lock (doesn't acquire lock)
func (c *Config) saveConfigUpdatesLocked() error {
	return c.saveConfigUpdatesWithValues(c.configPath, c.yamlConfig)
}

// saveConfigUpdatesWithValues saves configuration updates to disk with provided values
func (c *Config) saveConfigUpdatesWithValues(configPath string, yamlConfig *models.YamlConfig) error {
	if configPath == "" {
		// Use default config path if not set
		configPath = utils.ConfigYAMLPath
	}

	if yamlConfig == nil {
		// Create default YAML config if it doesn't exist
		logging.LogDebug(setConfigModuleName, "YAML config not loaded, creating default config")
		yamlConfig = c.createDefaultYamlConfig()
		if yamlConfig == nil {
			logging.LogWarn(setConfigModuleName, "Failed to create default YAML config, skipping save")
			return nil
		}
		// Set the YAML config and path in the instance
		// Note: We don't acquire lock here because this is called from saveConfigUpdatesLocked
		// which is called while SetConfig already holds the lock
		c.yamlConfig = yamlConfig
		if c.configPath == "" {
			c.configPath = configPath
		}
	}

	// Ensure current profile exists
	profile := yamlConfig.GetProfile(c.currentProfile.FullValue())
	if profile == nil {
		// Create default profile
		profile = models.NewProfileConfig()
		yamlConfig.Profiles[c.currentProfile.FullValue()] = profile
	}

	// Save to file (use SaveConfigWithYaml to avoid lock acquisition)
	if err := SaveConfigWithYaml(configPath, yamlConfig); err != nil {
		return fmt.Errorf("failed to save config to %s: %w", configPath, err)
	}

	logging.LogDebug(setConfigModuleName, fmt.Sprintf("Config updates saved to %s", configPath))
	return nil
}

// createDefaultYamlConfig creates a default YAML config structure
// Note: Must be called while holding the config lock (doesn't acquire lock)
func (c *Config) createDefaultYamlConfig() *models.YamlConfig {
	currentProfile := c.currentProfile

	if currentProfile == "" {
		currentProfile = utils.ConfigSwitcherStateDefault
	}

	yamlConfig := models.NewYamlConfig()
	yamlConfig.CurrentProfile = currentProfile.FullValue()

	// Create default profile with default values
	defaultProfile := models.NewProfileConfig()
	defaultProfile.SetProperty("controllerUrl", "http://localhost:54421/api/v3/")
	defaultProfile.SetProperty("controllerCert", "/etc/edgelet/cert.crt")
	defaultProfile.SetProperty("networkInterface", "dynamic")
	defaultProfile.SetProperty("dockerUrl", "unix:///var/run/docker.sock")
	defaultProfile.SetProperty("diskDirectory", "/var/lib/edgelet/")
	defaultProfile.SetProperty("diskLimit", "10")
	defaultProfile.SetProperty("memoryLimit", "4096")
	defaultProfile.SetProperty("cpuLimit", "80")
	defaultProfile.SetProperty("logDirectory", "/var/log/edgelet/")
	defaultProfile.SetProperty("logLimit", "10")
	defaultProfile.SetProperty("logFileCount", "10")
	defaultProfile.SetProperty("logLevel", "INFO")
	defaultProfile.SetProperty("statusFrequency", "10")
	defaultProfile.SetProperty("changeFrequency", "20")
	defaultProfile.SetProperty("deviceScanFrequency", "60")
	defaultProfile.SetProperty("watchdogEnabled", "off")
	defaultProfile.SetProperty("edgeGuardFrequency", "0")
	defaultProfile.SetProperty("gpsDevice", "/dev/ttyUSB0")
	defaultProfile.SetProperty("gpsScanFrequency", "60")
	defaultProfile.SetProperty("gpsMode", "auto")
	defaultProfile.SetProperty("gpsCoordinates", "0,0")
	defaultProfile.SetProperty("arch", "auto")
	defaultProfile.SetProperty("secureMode", "off")
	defaultProfile.SetProperty("dockerPruningFrequency", "1")
	defaultProfile.SetProperty("availableDiskThreshold", "20")
	defaultProfile.SetProperty("upgradeScanFrequency", "24")
	defaultProfile.SetProperty("devMode", "off")
	defaultProfile.SetProperty("timeZone", "")
	defaultProfile.SetProperty("namespace", "default")

	yamlConfig.Profiles[currentProfile.FullValue()] = defaultProfile

	return yamlConfig
}

func normalizeGPSCoordinates(value string) (string, error) {
	parts := strings.Split(strings.TrimSpace(value), ",")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid GPS coordinates format: expected lat,lon")
	}

	lat, err := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	if err != nil {
		return "", fmt.Errorf("invalid latitude: %w", err)
	}
	lon, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err != nil {
		return "", fmt.Errorf("invalid longitude: %w", err)
	}
	if lat < -90 || lat > 90 {
		return "", fmt.Errorf("latitude must be between -90 and 90")
	}
	if lon < -180 || lon > 180 {
		return "", fmt.Errorf("longitude must be between -180 and 180")
	}

	return fmt.Sprintf("%.5f,%.5f", lat, lon), nil
}
