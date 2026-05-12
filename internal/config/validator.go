package config

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/eclipse-iofog/agent/internal/buildmeta"
	"github.com/eclipse-iofog/agent/internal/constants"
	"github.com/eclipse-iofog/agent/internal/utils"
)

// ValidateConfig validates the configuration
func ValidateConfig(cfg *Config) error {
	var errors []string

	// Validate disk limit
	if cfg.DiskLimit < 0.5 || cfg.DiskLimit > utils.MaxDiskConsumptionLimit {
		errors = append(errors, fmt.Sprintf("disk limit must be between 0.5 and %.1f GB", utils.MaxDiskConsumptionLimit))
	}

	// Validate memory limit
	if cfg.MemoryLimit < 128 || cfg.MemoryLimit > 1048576 {
		errors = append(errors, "memory limit must be between 128 and 1048576 MB")
	}

	// Validate CPU limit
	if cfg.CPULimit < 5 || cfg.CPULimit > 100 {
		errors = append(errors, "CPU limit must be between 5% and 100%")
	}

	// Validate log disk limit
	if cfg.LogDiskLimit < 0.5 || cfg.LogDiskLimit > utils.MaxDiskConsumptionLimit {
		errors = append(errors, fmt.Sprintf("log disk limit must be between 0.5 and %.1f GB", utils.MaxDiskConsumptionLimit))
	}

	// Validate log file count
	if cfg.LogFileCount < 1 || cfg.LogFileCount > 100 {
		errors = append(errors, "log file count must be between 1 and 100")
	}

	// Validate log level
	validLogLevels := map[string]bool{
		"DEBUG": true,
		"INFO":  true,
		"WARN":  true,
		"ERROR": true,
		"FATAL": true,
		"OFF":   true,
	}
	if !validLogLevels[strings.ToUpper(cfg.LogLevel)] {
		errors = append(errors, "log level must be one of: DEBUG, INFO, WARN, ERROR, FATAL, OFF")
	}

	// Validate frequencies
	if cfg.StatusFrequency < 1 {
		errors = append(errors, "status frequency must be greater than 0")
	}
	if cfg.ChangeFrequency < 1 {
		errors = append(errors, "change frequency must be greater than 0")
	}
	if cfg.DeviceScanFrequency < 1 {
		errors = append(errors, "device scan frequency must be greater than 0")
	}
	if cfg.PostDiagnosticsFreq < 1 {
		errors = append(errors, "post diagnostics frequency must be greater than 0")
	}

	// Validate edge guard frequency
	if cfg.EdgeGuardFrequency < 0 {
		errors = append(errors, "edge guard frequency must be positive")
	}

	// Validate GPS scan frequency
	if cfg.GPSScanFrequency < 0 {
		errors = append(errors, "GPS scan frequency must be positive")
	}
	if cfg.GPSMode != "" {
		mode := strings.ToLower(strings.TrimSpace(cfg.GPSMode))
		switch mode {
		case "auto", "dynamic", "manual", "off":
		default:
			errors = append(errors, "GPS mode must be one of: auto, dynamic, manual, off")
		}
	}
	if strings.TrimSpace(cfg.GPSCoordinates) != "" {
		if _, err := normalizeGPSCoordinates(cfg.GPSCoordinates); err != nil {
			errors = append(errors, fmt.Sprintf("GPS coordinates are invalid: %v", err))
		}
	}

	// Validate docker URL
	if cfg.DockerURL != "" {
		if !strings.HasPrefix(cfg.DockerURL, "tcp://") && !strings.HasPrefix(cfg.DockerURL, "unix://") {
			errors = append(errors, "docker URL must start with 'tcp://' or 'unix://'")
		}
	}

	// Validate container engine (binary flavor restricts allowed engines)
	eng := strings.ToLower(strings.TrimSpace(cfg.ContainerEngine))
	if buildmeta.IsLite() {
		if eng != constants.EngineDocker && eng != constants.EnginePodman {
			errors = append(errors, fmt.Sprintf("this agent build (flavor=lite) only supports containerEngine: docker, podman (got %q)", cfg.ContainerEngine))
		}
	}
	if buildmeta.IsFull() {
		if eng != constants.EngineIofog {
			errors = append(errors, fmt.Sprintf("this agent build (flavor=full) requires containerEngine: iofog (got %q)", cfg.ContainerEngine))
		}
	}
	if eng != constants.EngineDocker && eng != constants.EnginePodman && eng != constants.EngineIofog {
		errors = append(errors, "containerEngine must be one of: docker, podman, iofog")
	}

	// dockerUrl rules per engine
	if eng == constants.EngineIofog {
		want := constants.IofogEngineDockerURL()
		if cfg.DockerURL != want {
			errors = append(errors, fmt.Sprintf("dockerUrl for containerEngine iofog must be %q (got %q)", want, cfg.DockerURL))
		}
	}
	if eng == constants.EnginePodman && strings.TrimSpace(cfg.DockerURL) == "" {
		errors = append(errors, "dockerUrl is required when containerEngine is podman (e.g. unix:///run/podman/podman.sock)")
	}

	// Validate shutdown grace period
	if cfg.ShutdownGracePeriodSeconds < 5 || cfg.ShutdownGracePeriodSeconds > 600 {
		errors = append(errors, "shutdownGracePeriodSeconds must be between 5 and 600")
	}

	// Validate controller timeouts (edge-friendly)
	if cfg.ControllerRequestTimeoutSeconds < 5 || cfg.ControllerRequestTimeoutSeconds > 300 {
		errors = append(errors, "controllerRequestTimeoutSeconds must be between 5 and 300")
	}
	if cfg.ControllerPingTimeoutSeconds < 5 || cfg.ControllerPingTimeoutSeconds > 300 {
		errors = append(errors, "controllerPingTimeoutSeconds must be between 5 and 300")
	}

	if len(errors) > 0 {
		return fmt.Errorf("validation errors: %s", strings.Join(errors, "; "))
	}

	return nil
}

// ValidateProperty validates a single property value
func ValidateProperty(key, value string) error {
	switch key {
	case "diskLimit":
		val, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return fmt.Errorf("invalid disk limit value: %w", err)
		}
		if val < 0.5 || val > utils.MaxDiskConsumptionLimit {
			return fmt.Errorf("disk limit must be between 0.5 and %.1f GB", utils.MaxDiskConsumptionLimit)
		}
	case "memoryLimit":
		val, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return fmt.Errorf("invalid memory limit value: %w", err)
		}
		if val < 128 || val > 1048576 {
			return fmt.Errorf("memory limit must be between 128 and 1048576 MB")
		}
	case "cpuLimit":
		val, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return fmt.Errorf("invalid CPU limit value: %w", err)
		}
		if val < 5 || val > 100 {
			return fmt.Errorf("CPU limit must be between 5%% and 100%%")
		}
	case "logLimit":
		val, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return fmt.Errorf("invalid log disk limit value: %w", err)
		}
		if val < 0.5 || val > utils.MaxDiskConsumptionLimit {
			return fmt.Errorf("log disk limit must be between 0.5 and %.1f GB", utils.MaxDiskConsumptionLimit)
		}
	case "logFileCount":
		val, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid log file count value: %w", err)
		}
		if val < 1 || val > 100 {
			return fmt.Errorf("log file count must be between 1 and 100")
		}
	case "logLevel":
		validLogLevels := map[string]bool{
			"DEBUG": true,
			"INFO":  true,
			"WARN":  true,
			"ERROR": true,
			"FATAL": true,
			"OFF":   true,
		}
		if !validLogLevels[strings.ToUpper(value)] {
			return fmt.Errorf("log level must be one of: DEBUG, INFO, WARN, ERROR, FATAL, OFF")
		}
	case "statusFrequency", "changeFrequency", "deviceScanFrequency", "postDiagnosticsFreq":
		val, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid frequency value: %w", err)
		}
		if val < 1 {
			return fmt.Errorf("frequency must be greater than 0")
		}
	case "edgeGuardFrequency", "gpsScanFrequency":
		val, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid frequency value: %w", err)
		}
		if val < 0 {
			return fmt.Errorf("frequency must be positive")
		}
	case "gpsMode":
		mode := strings.ToLower(strings.TrimSpace(value))
		switch mode {
		case "auto", "dynamic", "manual", "off":
		default:
			return fmt.Errorf("GPS mode must be one of: auto, dynamic, manual, off")
		}
	case "gpsCoordinates":
		if _, err := normalizeGPSCoordinates(value); err != nil {
			return err
		}
	case "dockerUrl":
		if value != "" && !strings.HasPrefix(value, "tcp://") && !strings.HasPrefix(value, "unix://") {
			return fmt.Errorf("docker URL must start with 'tcp://' or 'unix://'")
		}
	}

	return nil
}
