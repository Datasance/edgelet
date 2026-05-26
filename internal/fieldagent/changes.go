package fieldagent

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/datasance/edgelet/internal/config"
	"github.com/datasance/edgelet/internal/proxy"
	"github.com/datasance/edgelet/internal/utils"
	"github.com/datasance/edgelet/internal/utils/logging"
	"github.com/datasance/edgelet/internal/version"
)

// processChanges processes changes from the controller
func (fa *FieldAgent) processChanges(changes map[string]interface{}) bool {
	logging.LogDebug(moduleName, fmt.Sprintf("Starting processChanges with changes: %+v", changes))

	resetChanges := true
	initialization := fa.state.IsInitialization()
	logging.LogDebug(moduleName, fmt.Sprintf("Processing changes with initialization flag: %v", initialization))

	// Process deleteNode change
	if deleteNode, ok := changes["deleteNode"].(bool); ok && deleteNode && !initialization {
		logging.LogDebug(moduleName, "Processing deleteNode change")
		if err := fa.deleteNode(); err != nil {
			logging.LogError(moduleName, "Unable to delete node", err)
			resetChanges = false
		}
	} else {
		// Process other changes

		// Process reboot change
		if reboot, ok := changes["reboot"].(bool); ok && reboot && !initialization {
			logging.LogDebug(moduleName, "Processing reboot change")
			if err := fa.reboot(); err != nil {
				logging.LogError(moduleName, "Unable to perform reboot", err)
				resetChanges = false
			}
		}

		// Process config change
		if configChange, ok := changes["config"].(bool); ok && configChange && !initialization {
			logging.LogDebug(moduleName, "Processing config change")
			if err := fa.getFogConfig(); err != nil {
				logging.LogError(moduleName, "Unable to get config", err)
				resetChanges = false
			}
		}

		// Process version change
		if version, ok := changes["version"].(bool); ok && version && !initialization {
			logging.LogDebug(moduleName, "Processing version change")
			if err := fa.changeVersion(); err != nil {
				logging.LogError(moduleName, "Unable to change version", err)
				resetChanges = false
			}
		}

		// Process registries change
		if registries, ok := changes["registries"].(bool); ok && (registries || initialization) {
			logging.LogDebug(moduleName, "Processing registries change")
			if err := fa.loadRegistries(false); err != nil {
				logging.LogError(moduleName, "Unable to update registries", err)
				resetChanges = false
			}
		}

		// Process prune change
		if prune, ok := changes["prune"].(bool); ok && prune && !initialization {
			logging.LogDebug(moduleName, "Processing prune change")
			// DockerPruningManager would be called here
			// For now, just log
			logging.LogDebug(moduleName, "Docker prune requested")
		}

		// Process volumeMounts change
		if volumeMounts, ok := changes["volumeMounts"].(bool); ok && (volumeMounts || initialization) {
			logging.LogDebug(moduleName, "Processing volumeMounts change")
			if err := fa.loadVolumeMounts(); err != nil {
				logging.LogError(moduleName, "Unable to load volume mounts", err)
				resetChanges = false
			}
		}

		// Process microservice-related changes
		microserviceConfig, _ := changes["microserviceConfig"].(bool)
		microserviceList, _ := changes["microserviceList"].(bool)
		execSessions, _ := changes["execSessions"].(bool)

		if microserviceConfig || microserviceList || execSessions || initialization {
			logging.LogDebug(moduleName, fmt.Sprintf("Processing microservice related changes - microserviceConfig: %v, microserviceList: %v, execSessions: %v",
				microserviceConfig, microserviceList, execSessions))

			// Load microservices
			microservices, err := fa.loadMicroservices(false)
			if err != nil {
				logging.LogError(moduleName, "Unable to get microservices list", err)
				resetChanges = false
			} else {
				logging.LogDebug(moduleName, fmt.Sprintf("Loaded %d microservices", len(microservices)))

				// Process microservice config changes
				if microserviceConfig {
					logging.LogDebug(moduleName, "Processing microservice config changes")
					if err := fa.processMicroserviceConfig(microservices); err != nil {
						logging.LogError(moduleName, "Unable to update microservices config", err)
						resetChanges = false
					}
				}

				// Process exec sessions changes
				if execSessions {
					logging.LogDebug(moduleName, "Processing exec sessions changes")
					fa.HandleExecSessions(fa.GetLatestMicroservices())
				}
			}
		}

		// Process tunnel change
		if tunnel, ok := changes["tunnel"].(bool); ok && tunnel && !initialization {
			logging.LogDebug(moduleName, "Processing tunnel change")
			if err := fa.updateTunnel(); err != nil {
				logging.LogError(moduleName, "Unable to update tunnel", err)
				resetChanges = false
			}
		}

		// Process log sessions changes
		microserviceLogs, _ := changes["microserviceLogs"].(bool)
		fogLogs, _ := changes["fogLogs"].(bool)
		if microserviceLogs || fogLogs {
			logging.LogDebug(moduleName, fmt.Sprintf("Processing log sessions changes - microserviceLogs: %v, fogLogs: %v", microserviceLogs, fogLogs))
			// Fetch and handle log sessions (matching Java: handleLogSessions())
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			logSessionManager := GetLogSessionManager()
			sessions, err := logSessionManager.FetchLogSessions(ctx)
			cancel()
			if err != nil {
				logging.LogError(moduleName, "Unable to handle log sessions", err)
				resetChanges = false
			} else {
				logSessionManager.HandleLogSessions(sessions)
			}
		}
	}

	logging.LogDebug(moduleName, fmt.Sprintf("Finished processing changes with resetChanges: %v", resetChanges))
	return resetChanges
}

// deleteNode deletes the current fog node from controller and deprovisions
func (fa *FieldAgent) deleteNode() error {
	logging.LogDebug(moduleName, "start deleting current fog node from controller and make it deprovision")

	ctx, cancel := context.WithTimeout(fa.ctx, 30*time.Second)
	err := fa.apiClient.Delete(ctx, "delete-node")
	cancel()

	if err != nil {
		logging.LogError(moduleName, "Can't send delete node command", err)
	}

	if depErr := fa.Deprovision(true); depErr != nil {
		logging.LogWarn(moduleName, fmt.Sprintf("Deprovision after node deletion failed: %v", depErr))
	}
	logging.LogDebug(moduleName, "Finish deleting current fog node from controller and make it deprovision")
	return err
}

// reboot performs a remote reboot (placeholder - would need system commands)
func (fa *FieldAgent) reboot() error {
	logging.LogInfo(moduleName, "start Remote reboot of Linux machine from IOFog controller")
	// Execute reboot command (requires root privileges)
	// Note: This is a dangerous operation and should be carefully controlled
	output, _, err := utils.ExecuteCommand("sudo reboot")
	if err != nil {
		logging.LogError(moduleName, "Failed to execute reboot command", err)
		return err
	}
	logging.LogInfo(moduleName, "Reboot command executed: "+output)
	logging.LogInfo(moduleName, "Finished Remote reboot of Linux machine from IOFog controller")
	return nil
}

// changeVersion changes the agent version (placeholder)
func (fa *FieldAgent) changeVersion() error {
	logging.LogInfo(moduleName, "Starting change version action")

	if fa.NotProvisioned() || !fa.IsControllerConnected(false) {
		return nil
	}

	ctx, cancel := context.WithTimeout(fa.ctx, 30*time.Second)
	result, err := fa.apiClient.Request(ctx, "version", GET, nil, nil)
	cancel()

	if err != nil {
		return fmt.Errorf("unable to get version command: %w", err)
	}

	// Extract version command from result
	if versionData, ok := result["versionCommand"].(map[string]interface{}); ok {
		versionHandler := version.GetInstance()
		if err := versionHandler.ChangeVersion(versionData); err != nil {
			return fmt.Errorf("failed to change version: %w", err)
		}
	} else {
		logging.LogDebug(moduleName, fmt.Sprintf("Version change result: %+v", result))
	}

	logging.LogInfo(moduleName, "Finished change version operation, received from ioFog controller")
	return nil
}

// updateTunnel updates SSH tunnel configuration
func (fa *FieldAgent) updateTunnel() error {
	logging.LogDebug(moduleName, "Updating SSH tunnel configuration")

	// Get proxy config from controller
	proxyConfig, err := fa.getProxyConfig()
	if err != nil {
		return fmt.Errorf("failed to get proxy config: %w", err)
	}

	if proxyConfig == nil {
		logging.LogDebug(moduleName, "No proxy config received, skipping tunnel update")
		return nil
	}

	// Update SSH proxy manager
	proxyManager := proxy.GetInstance()
	if err := proxyManager.Update(proxyConfig); err != nil {
		return fmt.Errorf("failed to update SSH proxy: %w", err)
	}

	logging.LogDebug(moduleName, "SSH tunnel configuration updated successfully")
	return nil
}

// getProxyConfig gets proxy configuration from controller
func (fa *FieldAgent) getProxyConfig() (map[string]interface{}, error) {
	if fa.NotProvisioned() || !fa.IsControllerConnected(false) {
		return nil, nil
	}

	// Request tunnel config from controller
	ctx, cancel := context.WithTimeout(fa.ctx, 30*time.Second)
	defer cancel()

	response, err := fa.apiClient.Request(ctx, "tunnel", GET, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to request tunnel config: %w", err)
	}

	// Extract tunnel config from response
	if tunnelObj, ok := response["tunnel"].(map[string]interface{}); ok {
		return tunnelObj, nil
	}

	return nil, nil
}

// getFogConfig gets the fog configuration from controller
func (fa *FieldAgent) getFogConfig() error {
	logging.LogInfo(moduleName, "Starting Get ioFog config")

	if fa.NotProvisioned() || !fa.IsControllerConnected(false) {
		return nil
	}

	if fa.state.IsInitialization() {
		// Post config instead
		return fa.postFogConfig()
	}

	ctx, cancel := context.WithTimeout(fa.ctx, 30*time.Second)
	configs, err := fa.apiClient.Request(ctx, "config", GET, nil, nil)
	cancel()

	if err != nil {
		return fmt.Errorf("unable to get ioFog config: %w", err)
	}

	// Check for nested config objects (instance-config or agent-config)
	if instanceConfig, ok := configs["instance-config"].(map[string]interface{}); ok {
		configs = instanceConfig
	} else if agentConfig, ok := configs["agent-config"].(map[string]interface{}); ok {
		configs = agentConfig
	}

	// Map controller config keys to agent config keys (short codes)
	configMap := make(map[string]interface{})

	// Mapping from controller JSON keys to internal short codes
	keyMapping := map[string]string{
		"diskConsumptionLimit":      "d",
		"diskDirectory":             "dl",
		"memoryConsumptionLimit":    "m",
		"processorConsumptionLimit": "p",
		"controllerUrl":             "a",
		"controllerCert":            "ac",
		"dockerUrl":                 "c",
		"networkInterface":          "n",
		"logDiskConsumptionLimit":   "l",
		"logDiskDirectory":          "ld",
		"logFileCount":              "lc",
		"logLevel":                  "ll",
		"statusFrequency":           "sf",
		"changeFrequency":           "cf",
		"deviceScanFrequency":       "sd",
		"watchdogEnabled":           "idc",
		"edgeGuardFrequency":        "egf",
		"gpsMode":                   "gps",
		"gpsDevice":                 "gpsd",
		"gpsScanFrequency":          "gpsf",
		"arch":                      "ft",
		"secureMode":                "sec",
		"dockerPruningFrequency":    "pf",
		"availableDiskThreshold":    "dt",
		"upgradeScanFrequency":      "uf",
		"devMode":                   "dev",
		"timeZone":                  "tz",
	}

	for k, v := range configs {
		if shortKey, ok := keyMapping[k]; ok {
			configMap[shortKey] = v
		}
	}

	// Apply configuration
	if len(configMap) > 0 {
		logging.LogDebug(moduleName, fmt.Sprintf("Applying config changes: %+v", configMap))
		cfg := config.GetInstance()
		errorMap := cfg.SetConfig(configMap)

		if len(errorMap) > 0 {
			logging.LogError(moduleName, fmt.Sprintf("Errors applying config: %+v", errorMap), nil)
		} else {
			logging.LogInfo(moduleName, "Configuration applied successfully")
		}
	} else {
		logging.LogDebug(moduleName, "No matching config fields found to apply")
	}

	logging.LogDebug(moduleName, fmt.Sprintf("Config result: %+v", configs))
	logging.LogInfo(moduleName, "Finished Get ioFog config")
	return nil
}

// postFogConfig posts the fog configuration to controller
// Matching Java: postFogConfig() method
func (fa *FieldAgent) postFogConfig() error {
	logging.LogDebug(moduleName, "Post ioFog config")

	// Check if provisioned and connected (matching Java: notProvisioned() || !isControllerConnected(false))
	if fa.NotProvisioned() || !fa.IsControllerConnected(false) {
		logging.LogDebug(moduleName, "Skipping postFogConfig: not provisioned or not connected")
		return nil
	}

	cfg := config.GetInstance()

	// Parse GPS coordinates (matching Java logic)
	latitude := 0.0
	longitude := 0.0
	if cfg.GPSCoordinates != "" {
		coords := strings.Split(cfg.GPSCoordinates, ",")
		if len(coords) == 2 {
			if lat, err := strconv.ParseFloat(strings.TrimSpace(coords[0]), 64); err == nil {
				latitude = lat
			}
			if lon, err := strconv.ParseFloat(strings.TrimSpace(coords[1]), 64); err == nil {
				longitude = lon
			}
		}
	}

	// Get network interface name
	networkInterfaceName := cfg.NetworkInterface
	if networkInterfaceName == "" {
		networkInterfaceName = "UNKNOWN"
	}

	// Build config data matching Java JsonObject structure
	configData := map[string]interface{}{
		"networkInterface":          networkInterfaceName,
		"dockerUrl":                 cfg.DockerURL,
		"diskConsumptionLimit":      cfg.DiskLimit,
		"diskDirectory":             cfg.DiskDirectory,
		"memoryConsumptionLimit":    cfg.MemoryLimit,
		"processorConsumptionLimit": cfg.CPULimit,
		"logDiskConsumptionLimit":   cfg.LogDiskLimit,
		"logDiskDirectory":          cfg.LogDiskDirectory,
		"logFileCount":              cfg.LogFileCount,
		"statusFrequency":           cfg.StatusFrequency,
		"changeFrequency":           cfg.ChangeFrequency,
		"deviceScanFrequency":       cfg.DeviceScanFrequency,
		"watchdogEnabled":           cfg.WatchdogEnabled,
		"edgeGuardFrequency":        cfg.EdgeGuardFrequency,
		"gpsDevice":                 cfg.GPSDevice,
		"gpsScanFrequency":          cfg.GPSScanFrequency,
		"gpsMode":                   strings.ToLower(cfg.GPSMode),
		"latitude":                  latitude,
		"longitude":                 longitude,
		"logLevel":                  strings.ToUpper(cfg.LogLevel),
		"availableDiskThreshold":    cfg.AvailableDiskThreshold,
		"dockerPruningFrequency":    cfg.DockerPruningFrequency,
		"upgradeScanFrequency":      cfg.UpgradeScanFrequency,
	}

	// Use context from FieldAgent (daemon mode) or create new one (CLI mode)
	var ctx context.Context
	var cancel context.CancelFunc
	if fa.ctx != nil {
		ctx, cancel = context.WithTimeout(fa.ctx, 30*time.Second)
	} else {
		ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
	}
	defer cancel()

	// Post config using PATCH method (matching Java: RequestType.PATCH)
	_, err := fa.apiClient.Request(ctx, "config", PATCH, nil, configData)
	if err != nil {
		logging.LogError(moduleName, "Failed to post fog config to controller", err)
		return err
	}

	logging.LogInfo(moduleName, "Fog config posted to controller successfully")
	return nil
}

// InstanceGPSConfigUpdated sends dedicated GPS config updates to the controller.
// Matching Java: instanceGpsConfigUpdated() method.
func (fa *FieldAgent) InstanceGPSConfigUpdated() error {
	logging.LogDebug(moduleName, "Start ioFog GPS configuration update")
	if err := fa.postGPSConfig(); err != nil {
		logging.LogError(moduleName, "Error posting updated GPS config", err)
		return err
	}
	logging.LogDebug(moduleName, "Finished ioFog GPS configuration update")
	return nil
}

// postGPSConfig posts dedicated GPS coordinates to controller.
// Matching Java: postGpsConfig() method.
func (fa *FieldAgent) postGPSConfig() error {
	logging.LogDebug(moduleName, "Post ioFog GPS config")

	// Check if provisioned and connected (matching Java guard)
	if fa.NotProvisioned() || !fa.IsControllerConnected(false) {
		logging.LogDebug(moduleName, "Skipping postGPSConfig: not provisioned or not connected")
		return nil
	}

	latitude, longitude, ok := parseGPSCoordinates(config.GetInstance().GPSCoordinates)
	if !ok {
		logging.LogWarn(moduleName, "Skipping postGPSConfig due to invalid or empty gpsCoordinates")
		return nil
	}

	body := map[string]interface{}{
		"latitude":  latitude,
		"longitude": longitude,
	}

	// Use context from FieldAgent (daemon mode) or create new one (CLI mode).
	var ctx context.Context
	var cancel context.CancelFunc
	if fa.ctx != nil {
		ctx, cancel = context.WithTimeout(fa.ctx, 30*time.Second)
	} else {
		ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
	}
	defer cancel()

	_, err := fa.apiClient.Request(ctx, "config/gps", PATCH, nil, body)
	if err != nil {
		logging.LogError(moduleName, "Failed to post GPS config to controller", err)
		return err
	}

	logging.LogInfo(moduleName, "GPS config posted to controller successfully")
	return nil
}

func parseGPSCoordinates(raw string) (float64, float64, bool) {
	coords := strings.TrimSpace(raw)
	if coords == "" {
		return 0, 0, false
	}

	parts := strings.Split(coords, ",")
	if len(parts) != 2 {
		return 0, 0, false
	}

	lat, err := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	if err != nil {
		return 0, 0, false
	}
	lon, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err != nil {
		return 0, 0, false
	}

	return lat, lon, true
}
