package config

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/eclipse-iofog/agent-go/internal/utils"
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

	// Validate docker URL
	if cfg.DockerURL != "" {
		if !strings.HasPrefix(cfg.DockerURL, "tcp://") && !strings.HasPrefix(cfg.DockerURL, "unix://") {
			errors = append(errors, "docker URL must start with 'tcp://' or 'unix://'")
		}
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
	case "dockerUrl":
		if value != "" && !strings.HasPrefix(value, "tcp://") && !strings.HasPrefix(value, "unix://") {
			return fmt.Errorf("docker URL must start with 'tcp://' or 'unix://'")
		}
	}

	return nil
}
