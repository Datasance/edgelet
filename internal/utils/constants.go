package utils //nolint:revive // legacy package name

import (
	"os"
	"path/filepath"
)

// ModulesStatus represents the status of a module
type ModulesStatus string

const (
	ModulesStatusStarting ModulesStatus = "STARTING"
	ModulesStatusRunning  ModulesStatus = "RUNNING"
	ModulesStatusStopped  ModulesStatus = "STOPPED"
	ModulesStatusWarning  ModulesStatus = "WARNING"
)

// DockerStatus represents Docker daemon status
type DockerStatus string

const (
	DockerStatusNotPresent DockerStatus = "NOT_PRESENT"
	DockerStatusRunning    DockerStatus = "RUNNING"
	DockerStatusStopped    DockerStatus = "STOPPED"
)

// LinkStatus represents link status
type LinkStatus string

const (
	LinkStatusFailedVerification LinkStatus = "FAILED_VERIFICATION"
	LinkStatusFailedLogin        LinkStatus = "FAILED_LOGIN"
	LinkStatusConnected          LinkStatus = "CONNECTED"
)

// ConfigSwitcherState represents configuration profile state
type ConfigSwitcherState string

const (
	ConfigSwitcherStateDefault     ConfigSwitcherState = "default"
	ConfigSwitcherStateDevelopment ConfigSwitcherState = "development"
	ConfigSwitcherStateProduction  ConfigSwitcherState = "production"
)

// ParseConfigSwitcherState parses a string to ConfigSwitcherState
func ParseConfigSwitcherState(s string) (ConfigSwitcherState, error) {
	switch s {
	case "default", "def":
		return ConfigSwitcherStateDefault, nil
	case "development", "dev":
		return ConfigSwitcherStateDevelopment, nil
	case "production", "prod":
		return ConfigSwitcherStateProduction, nil
	default:
		return ConfigSwitcherStateDefault, ErrInvalidSwitcherState
	}
}

// FullValue returns the full value string for the state
func (s ConfigSwitcherState) FullValue() string {
	switch s {
	case ConfigSwitcherStateDevelopment:
		return "development"
	case ConfigSwitcherStateProduction:
		return "production"
	default:
		return "default"
	}
}

// Module indices
const (
	NumberOfModules = 7

	ResourceConsumptionManager = 0
	ProcessManager             = 1
	StatusReporter             = 2
	EdgeletAPI                 = 3
	FieldAgent                 = 4
	ResourceManager            = 5
	GPSManager                 = 6
)

// Size constants
const (
	KiB = 1024
	MiB = KiB * KiB
	GiB = KiB * KiB * KiB
)

// Environment variables
var (
	SNAP       = getEnvOrDefault("SNAP", "")
	SNAPCommon = getEnvOrDefault("SNAP_COMMON", "")
)

// Path constants
var (
	WindowsEdgeletPath   = getWindowsEdgeletBase()
	VarRun               = getVarRunPath()
	ConfigDir            = getConfigDir()
	EdgeletAPITokenPath  = ConfigDir + "edgelet-api"
	ConfigYAMLPath       = ConfigDir + "config.yaml"
	BackupConfigYAMLPath = ConfigDir + "config-bck.yaml"
)

// System constants
const (
	OSGroup                          = "edgelet"
	EdgeletDockerContainerNamePrefix = "edgelet_"
	MicroserviceFile                 = "microservices.json"
)

// Event constants
const (
	FieldAgentPingController                  = "FAPC"
	FieldAgentGetChangeList                   = "FACL"
	FieldAgentPostStatus                      = "FAPS"
	FieldAgentPostDiagnostic                  = "FAPD"
	StatusReporterSetStatusReporterSystemTime = "SRST"
	EdgeletAPIEvent                           = "LAPI"
	ResourceConsumptionManagerGetUsageData    = "RCUD"
	ProcessManagerContainersMonitor           = "PMCM"
	ProcessManagerCheckTasks                  = "PMCT"
	ResourceManagerGetUsageData               = "RMUD"
	EdgeletAPIControlWebSocketWorker          = "LACW"
	EdgeletAPIMessageWebSocketWorker          = "LAMW"
	ShutdownHook                              = "SDHK"
	SupervisorCheckEdgeletAPIStatus           = "SCLA"
	NetworkInterfaceManager                   = "INIM"
)

// Limits
const (
	MaxDiskConsumptionLimit = 100000.0
	PercentageCompletion    = 100.0
)

// Helper functions
func getEnvOrDefault(key, defaultValue string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultValue
}

func getWindowsEdgeletBase() string {
	if val := os.Getenv("EDGELET_PATH"); val != "" {
		return val
	}
	pd := os.Getenv("ProgramData")
	if pd == "" {
		pd = `C:\ProgramData`
	}
	return filepath.Join(pd, "Edgelet")
}

func getVarRunPath() string {
	if isWindows() {
		return filepath.Join(getWindowsEdgeletBase(), "run") + string(filepath.Separator)
	}
	return SNAPCommon + "/var/run/edgelet/"
}

func getConfigDir() string {
	if isWindows() {
		return filepath.Join(getWindowsEdgeletBase(), "config") + string(filepath.Separator)
	}
	return SNAPCommon + "/etc/edgelet/"
}

// GetConfigDir returns the config directory (can be called dynamically)
// This allows recalculation of config path when SNAP_COMMON changes (e.g., in dev environment)
func GetConfigDir() string {
	return getConfigDir()
}

func isWindows() bool {
	return os.Getenv("OS") == "Windows_NT" || os.Getenv("GOOS") == "windows"
}
