package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/eclipse-iofog/agent/internal/config"
	"github.com/eclipse-iofog/agent/internal/fieldagent"
	"github.com/eclipse-iofog/agent/internal/network"
	"github.com/eclipse-iofog/agent/internal/pruning"
	"github.com/eclipse-iofog/agent/internal/statusreporter"
	"github.com/eclipse-iofog/agent/internal/utils"
	"github.com/eclipse-iofog/agent/internal/utils/logging"
	versionpkg "github.com/eclipse-iofog/agent/internal/version"
)

const (
	cliParserModuleName = "CLI Parser"
)

// Note: version, buildTime, gitCommit are declared in commands.go

// ParseCommand parses and executes a CLI command (called from Local API)
func ParseCommand(command string) (string, error) {
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return "", fmt.Errorf("empty command")
	}

	action := parts[0]
	args := parts[1:]

	// Check for --help flag in args
	if len(args) > 0 && (args[0] == "--help" || args[0] == "-h" || args[0] == "-?") {
		return showCommandHelp(action), nil
	}

	// Route to appropriate action handler
	switch action {
	case "status":
		return parseStatusCommand(args)
	case "info":
		return parseInfoCommand(args)
	case "version":
		return parseVersionCommand(args)
	case "config":
		return parseConfigCommand(args)
	case "provision":
		return parseProvisionCommand(args)
	case "deprovision":
		return parseDeprovisionCommand(args)
	case "stop":
		return parseStopCommand(args)
	case "prune":
		return parsePruneCommand(args)
	case "cert":
		return parseCertCommand(args)
	case "switch":
		return parseSwitchCommand(args)
	case "help", "--help", "-h", "-?":
		return showHelpCLI(), nil
	default:
		return "", fmt.Errorf("unknown command: %s\n\nSee 'iofog-agent help' or 'iofog-agent <command> --help' for usage", action)
	}
}

func parseStatusCommand(_ []string) (string, error) {
	// Ensure config is loaded
	ensureConfigLoaded()

	// Get status report directly from StatusReporter
	statusReporter := statusreporter.GetInstance()
	report := statusReporter.GetStatusReport()
	return report + "\n", nil
}

func parseInfoCommand(_ []string) (string, error) {
	// Ensure config is loaded
	ensureConfigLoaded()

	// Get config report directly from Config
	cfg := config.GetInstance()

	// Get IP address directly from network manager (matching Java: IOFogNetworkInterfaceManager.getInstance().getCurrentIpAddress())
	// Java stores IP in memory only, not in config file
	ipAddress := "unable to retrieve ip address"
	networkManager := network.GetInstance()
	if networkManager != nil {
		ipAddr := networkManager.GetCurrentIPAddress()
		logging.LogDebug(cliParserModuleName, fmt.Sprintf("Network manager GetCurrentIPAddress() returned: '%s'", ipAddr))
		if ipAddr != "" {
			ipAddress = ipAddr
			logging.LogDebug(cliParserModuleName, fmt.Sprintf("Retrieved IP address from network manager: %s", ipAddr))
		} else {
			// Fallback: Try to update network interface if not initialized yet
			logging.LogDebug(cliParserModuleName, "Network manager returned empty IP address, attempting to update network interface")
			if err := networkManager.UpdateNetworkInterface(); err != nil {
				logging.LogWarn(cliParserModuleName, fmt.Sprintf("Failed to update network interface: %v", err))
			} else {
				// Try again after update
				ipAddr = networkManager.GetCurrentIPAddress()
				if ipAddr != "" {
					ipAddress = ipAddr
					logging.LogDebug(cliParserModuleName, fmt.Sprintf("Retrieved IP address after update: %s", ipAddr))
				} else {
					logging.LogWarn(cliParserModuleName, "Network manager still returned empty IP address after update")
				}
			}
		}
	} else {
		logging.LogWarn(cliParserModuleName, "Network manager instance is nil")
	}

	report := cfg.GetConfigReportWithIP(ipAddress)
	return report + "\n", nil
}

func parseVersionCommand(_ []string) (string, error) {
	// Get version directly from version package
	buildInfo := versionpkg.GetBuildInfo()
	return fmt.Sprintf("ioFog Agent %s (built %s, commit %s)\n", buildInfo["version"], buildInfo["buildTime"], buildInfo["gitCommit"]), nil
}

// ensureConfigLoaded ensures that config is loaded before CLI commands run
func ensureConfigLoaded() {
	cfg := config.GetInstance()
	// Try to load config if not already loaded
	if cfg.GetYamlConfig() == nil {
		// Try to load from default path
		if err := config.LoadConfig(utils.ConfigYAMLPath); err != nil {
			// If loading fails, create a default config structure
			logging.LogDebug(cliParserModuleName, "Config file not found, will create default on save")
		}
	}
}

func parseConfigCommand(args []string) (string, error) {
	if len(args) == 0 {
		return showHelpCLI(), nil
	}

	// Ensure config is loaded
	ensureConfigLoaded()

	cfg := config.GetInstance()
	var result strings.Builder

	// Check for defaults
	if len(args) == 1 && args[0] == "defaults" {
		// Reset to defaults - set all config params to defaults
		defaultsMap := map[string]interface{}{
			"d":  "10",
			"dl": "/var/lib/iofog-agent/",
			"m":  "512",
			"p":  "80",
			"a":  "http://localhost:54421/api/v3/",
			"c":  "unix:///var/run/docker.sock",
			"n":  "dynamic",
			"l":  "10",
			"ld": "/var/log/iofog-agent/",
			"lc": "10",
			"ll": "INFO",
			"sf": "10",
			"cf": "20",
			"df": "10",
			"sd": "60",
		}
		errors := cfg.SetConfig(defaultsMap)
		if len(errors) > 0 {
			return "", fmt.Errorf("error resetting configuration: %v", errors)
		}
		return "Configuration has been reset to its defaults.", nil
	}

	// Parse config parameters (format: -param value -param2 value2)
	configMap := make(map[string]interface{})
	i := 0
	for i < len(args) {
		param := args[i]

		// Remove leading dash if present
		param = strings.TrimPrefix(param, "-")

		// Special case: controllerCert can be empty
		if param == "ac" {
			configMap["ac"] = ""
			i++
		} else {
			if i+1 >= len(args) {
				return showHelpCLI(), nil
			}
			value := args[i+1]
			configMap[param] = value
			i += 2
		}
	}

	// Apply configuration changes using SetConfig
	errors := cfg.SetConfig(configMap)

	// Build result message
	for param, value := range configMap {
		if errMsg, hasError := errors[param]; hasError {
			result.WriteString(fmt.Sprintf("\n\tError : %s - %s", param, errMsg))
		} else {
			result.WriteString(fmt.Sprintf("\n\tChange accepted for Parameter : - %s, New Value is : %v", param, value))
		}
	}

	// Notify modules if config was updated successfully (async to not block)
	if len(errors) == 0 {
		go notifyModulesOfConfigChange()
	}

	return result.String() + "\n", nil
}

func parseProvisionCommand(args []string) (string, error) {
	if len(args) < 1 {
		return "", fmt.Errorf("provision command requires provisioning key")
	}

	provisionKey := args[0]
	fieldAgent := fieldagent.GetInstance()

	// Call FieldAgent provision
	err := fieldAgent.Provision(provisionKey)
	if err != nil {
		return "", fmt.Errorf("provisioning failed: %w", err)
	}

	cfg := config.GetInstance()
	uuid := cfg.IOFogUUID
	if uuid == "" {
		return fmt.Sprintf("Provisioning key: %s\nProvisioning completed but UUID not received\n", provisionKey), nil
	}

	// After successful provisioning, notify daemon to reload config if it's running
	// This ensures the daemon's FieldAgent gets the new credentials and can connect to controller
	client := NewClient()
	if client.IsDaemonRunning() {
		if err := notifyDaemonConfigReload(); err != nil {
			// Log warning but don't fail provisioning - config is saved, daemon will pick it up on next check
			logging.LogWarn(cliParserModuleName, fmt.Sprintf("Failed to notify daemon of config change: %v. Daemon will reload config on next SIGHUP or restart.", err))
		} else {
			logging.LogDebug(cliParserModuleName, "Successfully notified daemon to reload config after provisioning")
		}
	}

	return fmt.Sprintf("Provisioning key: %s\nProvisioning status: Success - UUID: %s\n", provisionKey, uuid), nil
}

// notifyDaemonConfigReload sends SIGHUP to the daemon process to trigger config reload
// This ensures the daemon's FieldAgent recreates its API client with new credentials
func notifyDaemonConfigReload() error {
	pidFile := filepath.Join(utils.VarRun, utils.PIDFileName)

	// Read PID from file
	pidBytes, err := os.ReadFile(pidFile) // #nosec G304 -- path computed from filepath.Join(constant dir, constant filename)
	if err != nil {
		return fmt.Errorf("failed to read PID file: %w", err)
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(pidBytes)))
	if err != nil {
		return fmt.Errorf("failed to parse PID: %w", err)
	}

	// Send SIGHUP signal to daemon
	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("failed to find daemon process: %w", err)
	}

	if err := process.Signal(syscall.SIGHUP); err != nil {
		return fmt.Errorf("failed to send SIGHUP to daemon: %w", err)
	}

	logging.LogDebug(cliParserModuleName, fmt.Sprintf("Sent SIGHUP to daemon (PID: %d) to reload config", pid))
	return nil
}

func parseDeprovisionCommand(_ []string) (string, error) {
	fieldAgent := fieldagent.GetInstance()

	// Call FieldAgent deprovision (clearCredentials = false to keep credentials)
	err := fieldAgent.Deprovision(false)
	if err != nil {
		return "", fmt.Errorf("deprovisioning failed: %w", err)
	}

	return "Deprovisioning status: Success\n", nil
}

func parseStopCommand(_ []string) (string, error) {
	// Stop command should be handled by supervisor
	// For now, return a message (actual stop is handled by supervisor)
	logging.LogInfo(cliParserModuleName, "Stop command received")
	return "Stopping ioFog Agent...\n", nil
}

func parsePruneCommand(_ []string) (string, error) {
	pruningManager := pruning.GetInstance()
	result := pruningManager.PruneAgent()
	return result + "\n", nil
}

func parseCertCommand(args []string) (string, error) {
	if len(args) < 1 {
		return "", fmt.Errorf("cert command requires base64-encoded certificate")
	}

	base64Cert := args[0]
	cfg := config.GetInstance()

	// Set certificate using SetConfig
	configMap := map[string]interface{}{
		"ac": base64Cert,
	}
	errors := cfg.SetConfig(configMap)
	if len(errors) > 0 {
		return "", fmt.Errorf("failed to set certificate: %v", errors)
	}

	return "Certificate successfully updated\n", nil
}

func parseSwitchCommand(args []string) (string, error) {
	if len(args) < 1 {
		return "", fmt.Errorf("switch command requires environment (dev|prod|def)")
	}

	env := args[0]
	cfg := config.GetInstance()

	// Parse environment string to ConfigSwitcherState
	profileState, err := utils.ParseConfigSwitcherState(env)
	if err != nil {
		return "", fmt.Errorf("invalid profile: %w", err)
	}

	// Switch config profile using SetCurrentProfile
	cfg.SetCurrentProfile(profileState)

	return fmt.Sprintf("Switched to %s profile\n", env), nil
}

func showHelpCLI() string {
	header := "\n" +
		"  _        __                                     _   \n" +
		" (_)      / _|                                   | |  \n" +
		"  _  ___ | |_ ___   __ _    __ _  __ _  ___ _ __ | |_ \n" +
		" | |/ _ \\|  _/ _ \\ / _` |  / _` |/ _` |/ _ \\ '_ \\| __|\n" +
		" | | (_) | || (_) | (_| | | (_| | (_| |  __/ | | | |_ \n" +
		" |_|\\___/|_| \\___/ \\__, |  \\__,_|\\__, |\\___|_| |_|\\__|\n" +
		"                    __/ |         __/ |               \n" +
		"                   |___/         |___/                \n" +
		"                                                                                \n" +
		"  Datasance PoT ioFog Agent vdev\n" +
		"  Command Line Interface\n" +
		"  =====================\n\n"

	return header +
		"Usage 1: iofog-agent [OPTION]\n" +
		"Usage 2: iofog-agent [COMMAND] <Argument>\n" +
		"Usage 3: iofog-agent [COMMAND] [Parameter] <Value>\n" +
		"\n" +
		"Option           GNU long option         Meaning\n" +
		"======           ===============         =======\n" +
		"-h, -?           --help                  Show this message\n" +
		"-v               --version               Display the software version and\n" +
		"                                         license information\n" +
		"\n" +
		"\n" +
		"Command          Arguments               Meaning\n" +
		"=======          =========               =======\n" +
		"help                                     Show this message\n" +
		"version                                  Display the software version and\n" +
		"                                         license information\n" +
		"status                                   Display current status information\n" +
		"                                         about the software\n" +
		"provision        <provisioning key>      Attach this software to the\n" +
		"                                         configured ioFog controller\n" +
		"deprovision                              Detach this software from all\n" +
		"                                         ioFog controllers\n" +
		"info                                     Display the current configuration\n" +
		"                                         and other information about the\n" +
		"                                         software\n" +
		"switch           <dev|prod|def>          Switch to different config\n" +
		"cert            <base64encodedcert>      Set the controller CA certificate\n" +
		"                                         for secure communication\n" +
		"config           [Parameter] [VALUE]     Change the software configuration\n" +
		"                                         according to the options provided\n" +
		"                 defaults                Reset configuration to default values\n" +
		"\n" +
		"\n" +
		"Report bugs to: developer@datasance.com\n" +
		"Datasance PoT docs: https://docs.datasance.com\n" +
		"For users with GitHub accounts, report bugs to: https://github.com/Datasance/Agent/issues\n"
}

// showCommandHelp shows help for a specific command
func showCommandHelp(command string) string {
	switch command {
	case "config":
		return showConfigHelp()
	case "provision":
		return showProvisionHelp()
	case "deprovision":
		return showDeprovisionHelp()
	case "status":
		return showStatusHelp()
	case "info":
		return showInfoHelp()
	case "cert":
		return showCertHelp()
	case "switch":
		return showSwitchHelp()
	case "prune":
		return showPruneHelp()
	case "stop":
		return showStopHelp()
	case "version":
		return showVersionHelp()
	default:
		return fmt.Sprintf("No specific help available for command: %s\n\nSee 'iofog-agent help' for general usage", command)
	}
}

// showConfigHelp shows help for config command
func showConfigHelp() string {
	return `Usage: iofog-agent config [PARAMETER] [VALUE]

Change agent configuration parameters.

Parameters:
  -d, diskLimit          Disk consumption limit (percentage, 1-100)
  -dl, diskDirectory     Disk directory path
  -m, memoryLimit        Memory consumption limit (MB)
  -p, cpuLimit           CPU consumption limit (percentage, 1-100)
  -a, controllerURL      Controller URL
  -ac, controllerCert    Controller CA certificate (base64 encoded)
  -c, dockerURL          Docker daemon URL
  -n, networkInterface   Network interface name or "dynamic"
  -l, logDiskLimit       Log disk consumption limit (percentage)
  -ld, logDiskDirectory  Log disk directory path
  -lc, logFileCount      Maximum number of log files
  -ll, logLevel          Log level (DEBUG, INFO, WARN, ERROR)
  -sf, statusFrequency   Status update frequency (seconds)
  -cf, changeFrequency   Change detection frequency (seconds)
  -df, diagnosticsFreq   Diagnostics frequency (seconds)
  -sd, deviceScanFreq     Device scan frequency (seconds)
  defaults                Reset all configuration to default values

Examples:
  iofog-agent config -d 50 -m 1024
  iofog-agent config -ll DEBUG
  iofog-agent config defaults

See 'iofog-agent help' for more information.
`
}

// showProvisionHelp shows help for provision command
func showProvisionHelp() string {
	return `Usage: iofog-agent provision <provisioning-key>

Attach this agent to the configured ioFog controller.

Arguments:
  provisioning-key    The provisioning key provided by the controller

Examples:
  iofog-agent provision abc123xyz789

See 'iofog-agent help' for more information.
`
}

// showDeprovisionHelp shows help for deprovision command
func showDeprovisionHelp() string {
	return `Usage: iofog-agent deprovision

Detach this agent from all ioFog controllers and clean up local data.

This command will:
  - Remove agent credentials
  - Stop all running microservices
  - Clear cached data (microservices, registries, edge resources)
  - Clear volume mounts

Examples:
  iofog-agent deprovision

See 'iofog-agent help' for more information.
`
}

// showStatusHelp shows help for status command
func showStatusHelp() string {
	return `Usage: iofog-agent status

Display current status information about the agent.

The status includes:
  - Daemon status (RUNNING/STOPPED)
  - Resource usage (memory, disk, CPU)
  - Running microservices count
  - Controller connection status
  - System information

Examples:
  iofog-agent status

See 'iofog-agent help' for more information.
`
}

// showInfoHelp shows help for info command
func showInfoHelp() string {
	return `Usage: iofog-agent info

Display the current configuration and other information about the agent.

The info includes:
  - Current configuration values
  - Network interface and IP address
  - System information

Examples:
  iofog-agent info

See 'iofog-agent help' for more information.
`
}

// showCertHelp shows help for cert command
func showCertHelp() string {
	return `Usage: iofog-agent cert <base64-encoded-certificate>

Set the controller CA certificate for secure communication.

Arguments:
  base64-encoded-certificate    The CA certificate in base64 format

Examples:
  iofog-agent cert LS0tLS1CRUdJTi...

See 'iofog-agent help' for more information.
`
}

// showSwitchHelp shows help for switch command
func showSwitchHelp() string {
	return `Usage: iofog-agent switch <environment>

Switch to a different configuration environment.

Arguments:
  environment    One of: dev, prod, def (default)

Examples:
  iofog-agent switch dev
  iofog-agent switch prod
  iofog-agent switch def

See 'iofog-agent help' for more information.
`
}

// showPruneHelp shows help for prune command
func showPruneHelp() string {
	return `Usage: iofog-agent prune

Clean up unused Docker resources (images, containers, volumes).

This command helps free up disk space by removing:
  - Unused Docker images
  - Stopped containers
  - Unused volumes

Examples:
  iofog-agent prune

See 'iofog-agent help' for more information.
`
}

// showStopHelp shows help for stop command
func showStopHelp() string {
	return `Usage: iofog-agent stop

Stop the ioFog agent daemon.

Examples:
  iofog-agent stop

See 'iofog-agent help' for more information.
`
}

// showVersionHelp shows help for version command
func showVersionHelp() string {
	return `Usage: iofog-agent version

Display the software version and build information.

Examples:
  iofog-agent version
  iofog-agent --version
  iofog-agent -v

See 'iofog-agent help' for more information.
`
}

// notifyModulesOfConfigChange notifies modules when configuration changes
func notifyModulesOfConfigChange() {
	// Trigger full config reload via Config callback (registered by Supervisor)
	if err := config.GetInstance().TriggerReloadCallback(); err != nil {
		logging.LogError(cliParserModuleName, "Failed to reload agent configuration", err)
	}
}
