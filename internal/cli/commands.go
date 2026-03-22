package cli

import (
	"fmt"
	"strings"
)

var (
	version   = "dev"
	buildTime = "unknown"
	gitCommit = "unknown"
)

// HandleCommand handles a CLI command
func HandleCommand(args []string) string {
	if len(args) == 0 {
		return showHelp()
	}

	command := args[0]
	client := NewClient()

	// Check if daemon is running for commands that need it
	needsDaemon := map[string]bool{
		"stop":        true,
		"status":      true,
		"info":        true,
		"provision":   true,
		"deprovision": true,
		"config":      true,
		"prune":       true,
		"cert":        true,
		"switch":      true,
	}

	if needsDaemon[command] && !client.IsDaemonRunning() {
		return "ioFog Agent is not running. Please start it first with 'iofog-agent start'."
	}

	switch command {
	case "start":
		return handleStart()
	case "stop":
		return handleStop(client, args)
	case "status":
		return handleStatus(client)
	case "info":
		return handleInfo(client)
	case "provision":
		return handleProvision(client, args)
	case "deprovision":
		return handleDeprovision(client)
	case "config":
		return handleConfig(client, args)
	case "version", "--version", "-v":
		return handleVersion()
	case "help", "--help", "-h", "-?":
		return showHelp()
	case "prune":
		return handlePrune(client)
	case "cert":
		return handleCert(client, args)
	case "switch":
		return handleSwitch(client, args)
	default:
		// Try to send to daemon
		cmdStr := strings.Join(args, " ")
		result, err := client.SendCommand(cmdStr)
		if err != nil {
			return fmt.Sprintf("Unknown command: %s\n\nSee 'iofog-agent help' or 'iofog-agent <command> --help' for usage\n\n%s", command, showHelp())
		}
		return result
	}
}

func handleStart() string {
	// Check if already running
	client := NewClient()
	if client.IsDaemonRunning() {
		return "ioFog Agent is already running."
	}

	// Start daemon (this would typically fork/exec the daemon)
	// For now, just return a message
	return "Starting ioFog Agent daemon...\nUse 'iofog-agentd start' to run the daemon directly."
}

func handleStop(client *Client, _ []string) string {
	// Send to daemon via Local API
	result, err := client.SendCommand("stop")
	if err != nil {
		return fmt.Sprintf("Error stopping daemon: %v", err)
	}
	return result
}

func handleStatus(client *Client) string {
	// Send to daemon via Local API
	result, err := client.SendCommand("status")
	if err != nil {
		return fmt.Sprintf("Error getting status: %v", err)
	}
	return result
}

func handleInfo(client *Client) string {
	// Send to daemon via Local API
	result, err := client.SendCommand("info")
	if err != nil {
		return fmt.Sprintf("Error getting info: %v", err)
	}
	return result
}

func handleProvision(client *Client, args []string) string {
	if len(args) < 2 {
		return "Error: provision command requires a provisioning key\n\nUsage: iofog-agent provision <provisioning-key>\n\nSee 'iofog-agent provision --help' for more information."
	}
	// Send to daemon via Local API
	cmd := strings.Join(args, " ")
	result, err := client.SendCommand(cmd)
	if err != nil {
		return fmt.Sprintf("Error provisioning: %v", err)
	}
	return result
}

func handleDeprovision(client *Client) string {
	// Send to daemon via Local API
	result, err := client.SendCommand("deprovision")
	if err != nil {
		return fmt.Sprintf("Error deprovisioning: %v", err)
	}
	return result
}

func handleConfig(client *Client, args []string) string {
	if len(args) == 1 {
		return "Error: config command requires parameters\n\nUsage: iofog-agent config [PARAMETER] [VALUE]\n\nSee 'iofog-agent config --help' for more information."
	}
	// Send to daemon via Local API
	cmd := strings.Join(args, " ")
	result, err := client.SendCommand(cmd)
	if err != nil {
		return fmt.Sprintf("Error updating config: %v", err)
	}
	return result
}

func handleVersion() string {
	return fmt.Sprintf("ioFog Agent %s (built %s, commit %s)", version, buildTime, gitCommit)
}

func handlePrune(client *Client) string {
	// Send to daemon via Local API
	result, err := client.SendCommand("prune")
	if err != nil {
		return fmt.Sprintf("Error pruning: %v", err)
	}
	return result
}

func handleCert(client *Client, args []string) string {
	if len(args) < 2 {
		return "Error: cert command requires a base64-encoded certificate\n\nUsage: iofog-agent cert <base64-encoded-certificate>\n\nSee 'iofog-agent cert --help' for more information."
	}
	// Send to daemon via Local API
	cmd := strings.Join(args, " ")
	result, err := client.SendCommand(cmd)
	if err != nil {
		return fmt.Sprintf("Error updating certificate: %v", err)
	}
	return result
}

func handleSwitch(client *Client, args []string) string {
	if len(args) < 2 {
		return "Error: switch command requires an environment\n\nUsage: iofog-agent switch <dev|prod|def>\n\nSee 'iofog-agent switch --help' for more information."
	}
	// Send to daemon via Local API
	cmd := strings.Join(args, " ")
	result, err := client.SendCommand(cmd)
	if err != nil {
		return fmt.Sprintf("Error switching config: %v", err)
	}
	return result
}

func showHelp() string {
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
		"  Datasance PoT ioFog Agent v" + version + "\n" +
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
		"For users with GitHub accounts, report bugs to: https://github.com/Datasance/Agent/issues"
}
