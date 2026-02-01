package config

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/eclipse-iofog/agent-go/internal/models"
	"github.com/eclipse-iofog/agent-go/internal/utils"
	"github.com/eclipse-iofog/agent-go/internal/utils/logging"
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
	"n":    "networkInterface",
	"l":    "logDiskLimit",
	"ld":   "logDiskDirectory",
	"lc":   "logFileCount",
	"ll":   "logLevel",
	"sf":   "statusFrequency",
	"cf":   "changeFrequency",
	"df":   "postDiagnosticsFreq",
	"sd":   "deviceScanFrequency",
	"idc":  "watchdogEnabled",
	"egf":  "edgeGuardFrequency",
	"gps":  "gpsMode",
	"gpsd": "gpsDevice",
	"gpsf": "gpsScanFrequency",
	"ft":   "arch",
	"sec":  "secureMode",
	"pf":   "dockerPruningFrequency",
	"dt":   "availableDiskThreshold",
	"uf":   "readyToUpgradeScanFrequency",
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
		if strings.HasPrefix(valueStr, "+") {
			valueStr = valueStr[1:]
		}

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

// setConfigField sets a specific config field value
func (c *Config) setConfigField(fieldName, value, option string) error {
	switch fieldName {
	case "diskLimit":
		val, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return fmt.Errorf("invalid disk limit: %w", err)
		}
		c.DiskLimit = val
		c.setYamlProperty("diskLimit", value)

	case "diskDirectory":
		c.DiskDirectory = value
		c.setYamlProperty("diskDirectory", value)

	case "memoryLimit":
		val, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return fmt.Errorf("invalid memory limit: %w", err)
		}
		c.MemoryLimit = val
		c.setYamlProperty("memoryLimit", value)

	case "cpuLimit":
		val, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return fmt.Errorf("invalid CPU limit: %w", err)
		}
		c.CPULimit = val
		c.setYamlProperty("cpuLimit", value)

	case "controllerURL":
		c.ControllerURL = value
		c.setYamlProperty("controllerUrl", value)

	case "controllerCert":
		c.ControllerCert = value
		c.setYamlProperty("controllerCert", value)

	case "dockerURL":
		c.DockerURL = value
		c.setYamlProperty("dockerUrl", value)

	case "networkInterface":
		c.NetworkInterface = value
		c.setYamlProperty("networkInterface", value)

	case "logDiskLimit":
		val, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return fmt.Errorf("invalid log disk limit: %w", err)
		}
		c.LogDiskLimit = val
		c.setYamlProperty("logDiskLimit", value)

	case "logDiskDirectory":
		c.LogDiskDirectory = value
		c.setYamlProperty("logDiskDirectory", value)

	case "logFileCount":
		val, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid log file count: %w", err)
		}
		c.LogFileCount = val
		c.setYamlProperty("logFileCount", value)

	case "logLevel":
		c.LogLevel = value
		c.setYamlProperty("logLevel", value)

	case "statusFrequency":
		val, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid status frequency: %w", err)
		}
		c.StatusFrequency = val
		c.setYamlProperty("statusFrequency", value)

	case "changeFrequency":
		val, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid change frequency: %w", err)
		}
		c.ChangeFrequency = val
		c.setYamlProperty("changeFrequency", value)

	case "postDiagnosticsFreq":
		val, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid post diagnostics frequency: %w", err)
		}
		c.PostDiagnosticsFreq = val
		c.setYamlProperty("postDiagnosticsFreq", value)

	case "deviceScanFrequency":
		val, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid device scan frequency: %w", err)
		}
		c.DeviceScanFrequency = val
		c.setYamlProperty("deviceScanFrequency", value)

	case "watchdogEnabled":
		c.WatchdogEnabled = strings.ToLower(value) != "off"
		c.setYamlProperty("watchdogEnabled", value)

	case "edgeGuardFrequency":
		val, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid edge guard frequency: %w", err)
		}
		c.EdgeGuardFrequency = val
		c.setYamlProperty("edgeGuardFrequency", value)

	case "gpsMode":
		c.GPSMode = value
		c.setYamlProperty("gpsMode", value)

	case "gpsDevice":
		c.GPSDevice = value
		c.setYamlProperty("gpsDevice", value)

	case "gpsScanFrequency":
		val, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid GPS scan frequency: %w", err)
		}
		c.GPSScanFrequency = val
		c.setYamlProperty("gpsScanFrequency", value)

	case "arch":
		c.Arch = value
		c.setYamlProperty("arch", value)

	case "secureMode":
		c.SecureMode = strings.ToLower(value) != "off"
		c.setYamlProperty("secureMode", value)

	case "dockerPruningFrequency":
		val, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid docker pruning frequency: %w", err)
		}
		c.DockerPruningFrequency = val
		c.setYamlProperty("dockerPruningFrequency", value)

	case "availableDiskThreshold":
		val, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid available disk threshold: %w", err)
		}
		c.AvailableDiskThreshold = val
		c.setYamlProperty("availableDiskThreshold", value)

	case "readyToUpgradeScanFrequency":
		val, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid ready to upgrade scan frequency: %w", err)
		}
		c.ReadyToUpgradeScanFrequency = val
		c.setYamlProperty("readyToUpgradeScanFrequency", value)

	case "devMode":
		c.DevMode = strings.ToLower(value) != "off"
		c.setYamlProperty("devMode", value)

	case "timeZone":
		c.TimeZone = value
		c.setYamlProperty("timeZone", value)

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
// Must be called while holding the config lock
func (c *Config) saveConfigUpdates() error {
	c.mu.RLock()
	configPath := c.configPath
	yamlConfig := c.yamlConfig
	c.mu.RUnlock()

	return c.saveConfigUpdatesWithValues(configPath, yamlConfig)
}

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
	defaultProfile.SetProperty("dockerPruningFrequency", "1")
	defaultProfile.SetProperty("availableDiskThreshold", "20")
	defaultProfile.SetProperty("readyToUpgradeScanFrequency", "24")
	defaultProfile.SetProperty("devMode", "off")
	defaultProfile.SetProperty("timeZone", "")
	defaultProfile.SetProperty("namespace", "default")

	yamlConfig.Profiles[currentProfile.FullValue()] = defaultProfile

	return yamlConfig
}

// notifyModulesOfConfigChangeAsync notifies modules asynchronously when configuration changes
// DEPRECATED: We now rely on file watcher to trigger reload via SIGHUP
func (c *Config) notifyModulesOfConfigChangeAsync() {
	// Trigger the reload via SIGHUP
	// logging.LogDebug(setConfigModuleName, "Sending SIGHUP to self to trigger config reload")
	// if err := syscall.Kill(os.Getpid(), syscall.SIGHUP); err != nil {
	// 	logging.LogError(setConfigModuleName, "Failed to send SIGHUP to self", err)
	// 	// Fallback to direct callback if signal fails (e.g. Windows)
	// 	if err := c.TriggerReloadCallback(); err != nil {
	// 		logging.LogError(setConfigModuleName, "Failed to trigger reload callback", err)
	// 	}
	// }
}
