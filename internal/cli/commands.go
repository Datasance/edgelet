package cli

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/gorilla/websocket"
	"golang.org/x/term"
	"gopkg.in/yaml.v3"
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

	if args[0] == "--help" || args[0] == "-h" || args[0] == "-?" {
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
		"ms":          true,
		"deploy":      true,
		"registry":    true,
		"system":      true,
		"auth":        true,
		"image":       true,
	}

	if needsDaemon[command] && !client.IsDaemonRunning() {
		return "ioFog Agent is not running. Please start it first with 'iofog-agent start'."
	}

	switch command {
	case "start":
		return handleStart()
	case "stop":
		return requestV3(client, "POST", "/v3/system/stop", nil)
	case "status":
		return requestV3(client, "GET", "/v3/system/status", nil)
	case "info":
		return requestV3(client, "GET", "/v3/system/info", nil)
	case "provision":
		return handleProvisionV3(client, args)
	case "deprovision":
		return handleDeprovisionV3(client, args[1:])
	case "config":
		return handleConfigV3(client, args)
	case "version", "--version", "-v":
		return handleVersion(client)
	case "help", "--help", "-h", "-?":
		return showHelp()
	case "prune":
		return handlePruneV3(client, args[1:])
	case "cert":
		return handleCert(client, args)
	case "switch":
		return handleSwitch(client, args)
	case "ms":
		return handleMicroserviceV3(client, args[1:])
	case "deploy":
		return handleDeployV3(client, args[1:])
	case "registry":
		return handleRegistryV3(client, args[1:])
	case "system":
		return handleSystemV3(client, args[1:])
	case "auth":
		return handleAuthV3(client, args[1:])
	case "image":
		return handleImageV3(client, args[1:])
	default:
		return fmt.Sprintf("Unknown command: %s\n\n%s", command, showHelp())
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

func handleProvisionV3(client *Client, args []string) string {
	if len(args) < 2 {
		return "Error: provision command requires a provisioning key\n\nUsage: iofog-agent provision <provisioning-key>\n\nSee 'iofog-agent provision --help' for more information."
	}
	result, err := client.RequestV3("POST", "/v3/system/provision", map[string]string{"provisioningKey": args[1]})
	if err != nil {
		return formatV3RequestError(err)
	}
	agentUUID := mapValueAsString(result, "agentUuid")
	if agentUUID == "<unknown>" {
		agentUUID = mapValueAsString(result, "iofogUuid")
	}
	if agentUUID == "<unknown>" {
		infoResult, infoErr := client.RequestV3("GET", "/v3/system/info", nil)
		if infoErr == nil {
			agentUUID = mapValueAsString(infoResult, "iofogUuid")
		}
	}
	return formatProvisionSuccess(agentUUID)
}

func handleDeprovisionV3(client *Client, args []string) string {
	scope := "all"
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--scope":
			if i+1 >= len(args) {
				return "Error[INVALID_ARGUMENT]: --scope requires all|local"
			}
			scope = strings.ToLower(strings.TrimSpace(args[i+1]))
			i++
		case "--keep-local":
			scope = "local"
		case "-h", "--help", "-?":
			return "Usage: iofog-agent deprovision [--scope all|local] [--keep-local]"
		default:
			return fmt.Sprintf("Error[INVALID_ARGUMENT]: unknown flag %s", args[i])
		}
	}
	if scope != "all" && scope != "local" {
		return "Error[INVALID_ARGUMENT]: --scope requires all|local"
	}
	path := "/v3/system/provision"
	if scope != "all" {
		path += "?scope=" + scope
	}
	_, err := client.RequestV3("DELETE", path, nil)
	if err != nil {
		return formatV3RequestError(err)
	}
	if scope == "local" {
		return "agent deprovisioned successfully; preserving local microservices"
	}
	return "agent deprovisioned successfully; started cleanup of managed and local microservices"
}

func formatProvisionSuccess(agentUUID string) string {
	if strings.TrimSpace(agentUUID) == "" || agentUUID == "<unknown>" {
		return "agent provisioned successfully"
	}
	return fmt.Sprintf("agent provisioned successfully (uuid: %s)", agentUUID)
}

func handleVersion(client *Client) string {
	daemonVersion, err := client.RequestV3("GET", "/v3/system/version", nil)
	return formatVersionOutput(version, buildTime, gitCommit, daemonVersion, err)
}

func handleCert(client *Client, args []string) string {
	if len(args) >= 2 && isHelpArg(args[1]) {
		return "Usage: iofog-agent cert <base64-encoded-certificate>\n\nDecodes and installs controller certificate to configured controllerCert path, then enables secure mode."
	}
	if len(args) < 2 {
		return "Error[INVALID_ARGUMENT]: cert command requires a base64-encoded certificate\n\nUsage: iofog-agent cert <base64-encoded-certificate>\n\nSee 'iofog-agent cert --help' for more information."
	}
	certValue := strings.TrimSpace(args[1])
	if certValue == "" {
		return "Error[INVALID_ARGUMENT]: certificate value cannot be empty"
	}
	return requestV3(client, "POST", "/v3/system/controller/cert", map[string]interface{}{
		"certificate": certValue,
	})
}

func handleSwitch(client *Client, args []string) string {
	if len(args) >= 2 && isHelpArg(args[1]) {
		return "Usage: iofog-agent switch <dev|prod|def>\n\nSwitches active configuration profile and reloads daemon configuration."
	}
	if len(args) < 2 {
		return "Error[INVALID_ARGUMENT]: switch command requires a profile\n\nUsage: iofog-agent switch <dev|prod|def>\n\nSee 'iofog-agent switch --help' for more information."
	}
	profile := strings.TrimSpace(args[1])
	switch profile {
	case "dev", "development", "prod", "production", "def", "default":
	default:
		return "Error[INVALID_ARGUMENT]: profile must be one of dev|prod|def"
	}
	return requestV3(client, "POST", "/v3/system/config/switch", map[string]interface{}{
		"profile": profile,
	})
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
		"Usage:\n" +
		"  iofog-agent [command]\n\n" +
		"Core commands:\n" +
		"  status | info | provision <key> | deprovision [--scope all|local] [--keep-local] | prune\n" +
		"  config <key> <value> [<key> <value> ...]\n" +
		"  config -n <iface> -a <controllerUrl> ...\n" +
		"  cert <base64-or-pem-certificate>\n" +
		"  switch <dev|prod|def>\n" +
		"  ms ps\n" +
		"  ms inspect <id>\n" +
		"  ms logs <id>\n" +
		"  ms exec <id> -- <command...>\n" +
		"  ms start|stop|restart|kill|rm <id>\n" +
		"  deploy -f <manifest.yaml>\n" +
		"  registry ls | inspect <id> | rm <id>\n" +
		"  image ls | pull <image> | load -f <path> | prune | rm <selector>\n" +
		"  auth whoami | auth tokens\n\n" +
		"Use 'iofog-agent <command> --help' for detailed usage.\n\n" +
		"Validation behavior:\n" +
		"  - config values are validated client-side before request dispatch.\n" +
		"  - errors are deterministic: Error[CODE]: message.\n" +
		"  - profile values for switch are limited to dev|prod|def.\n\n" +
		"Options:\n" +
		"  -h, --help        Show help\n" +
		"  -v, --version     Show version\n\n" +
		"Report bugs to: developer@datasance.com\n" +
		"Datasance PoT docs: https://docs.datasance.com\n" +
		"For users with GitHub accounts, report bugs to: https://github.com/Datasance/Agent/issues"
}

func handleSystemV3(client *Client, args []string) string {
	if len(args) == 0 {
		return "Usage: iofog-agent system <status|info|version|reload|prune [dangling|containers|volumes|all]>"
	}
	switch args[0] {
	case "status":
		return requestV3(client, "GET", "/v3/system/status", nil)
	case "info":
		return requestV3(client, "GET", "/v3/system/info", nil)
	case "version":
		return requestV3(client, "GET", "/v3/system/version", nil)
	case "reload":
		return requestV3(client, "POST", "/v3/system/reload", nil)
	case "prune":
		mode, errOut := parsePruneModeArgs(args[1:], "Usage: iofog-agent system prune [dangling|containers|volumes|all]")
		if errOut != "" {
			return errOut
		}
		path := "/v3/system/prune"
		if mode != "" {
			path += "?mode=" + mode
		}
		return requestV3(client, "POST", path, nil)
	default:
		return "Usage: iofog-agent system <status|info|version|reload|prune [dangling|containers|volumes|all]>"
	}
}

func handlePruneV3(client *Client, args []string) string {
	mode, errOut := parsePruneModeArgs(args, "Usage: iofog-agent prune [dangling|containers|volumes|all]")
	if errOut != "" {
		return errOut
	}
	path := "/v3/system/prune"
	if mode != "" {
		path += "?mode=" + mode
	}
	return requestV3(client, "POST", path, nil)
}

func parsePruneModeArgs(args []string, usage string) (string, string) {
	mode := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-h", "--help", "-?":
			return "", usage
		case "-m", "--mode":
			if i+1 >= len(args) {
				return "", "Error[INVALID_ARGUMENT]: --mode requires dangling|containers|volumes|all"
			}
			mode = strings.ToLower(strings.TrimSpace(args[i+1]))
			i++
		default:
			if strings.HasPrefix(args[i], "-") {
				return "", fmt.Sprintf("Error[INVALID_ARGUMENT]: unknown flag %s", args[i])
			}
			if mode != "" {
				return "", "Error[INVALID_ARGUMENT]: prune mode provided multiple times"
			}
			mode = strings.ToLower(strings.TrimSpace(args[i]))
		}
	}
	if mode == "" {
		return "", ""
	}
	switch mode {
	case "dangling", "containers", "volumes", "all":
		return mode, ""
	default:
		return "", "Error[INVALID_ARGUMENT]: prune mode must be dangling|containers|volumes|all"
	}
}

func handleConfigV3(client *Client, args []string) string {
	if len(args) >= 2 && isHelpArg(args[1]) {
		return showConfigHelpV3()
	}
	if len(args) < 2 {
		return showConfigHelpV3()
	}
	configArgs := args[1:]
	if strings.EqualFold(configArgs[0], "set") {
		configArgs = configArgs[1:]
	}
	if len(configArgs) > 0 && isHelpArg(configArgs[0]) {
		return showConfigHelpV3()
	}
	if len(configArgs) == 0 {
		return showConfigHelpV3()
	}
	setMap, err := parseConfigSetArgs(configArgs)
	if err != nil {
		return fmt.Sprintf("Error[INVALID_ARGUMENT]: %v\n\n%s", err, showConfigHelpV3())
	}
	payload := map[string]interface{}{"set": setMap}
	before, _ := client.RequestV3("GET", "/v3/system/config", nil)
	after, reqErr := client.RequestV3("PATCH", "/v3/system/config", payload)
	if reqErr != nil {
		return formatV3RequestError(reqErr)
	}
	return formatConfigMutationOutput(setMap, before, after)
}

func handleMicroserviceV3(client *Client, args []string) string {
	if len(args) > 0 && isHelpArg(args[0]) {
		return showMSHelpV3()
	}
	if len(args) == 0 {
		return showMSHelpV3()
	}
	switch args[0] {
	case "ps":
		source := "all"
		for i := 1; i < len(args); i++ {
			switch args[i] {
			case "--source":
				if i+1 >= len(args) {
					return "Error[INVALID_ARGUMENT]: --source requires managed|local|all"
				}
				source = strings.ToLower(strings.TrimSpace(args[i+1]))
				i++
			case "-h", "--help", "-?":
				return "Usage: iofog-agent ms ps [--source managed|local|all]"
			default:
				return fmt.Sprintf("Error[INVALID_ARGUMENT]: unknown flag %s", args[i])
			}
		}
		if source != "managed" && source != "local" && source != "all" {
			return "Error[INVALID_ARGUMENT]: --source requires managed|local|all"
		}
		return requestV3(client, "GET", "/v3/ms?source="+source, nil)
	case "inspect":
		if len(args) < 2 {
			return "Usage: iofog-agent ms inspect <id> [--summary]"
		}
		summary := false
		for i := 2; i < len(args); i++ {
			if args[i] == "--summary" {
				summary = true
				continue
			}
			return fmt.Sprintf("Error[INVALID_ARGUMENT]: unknown flag %s", args[i])
		}
		path := "/v3/ms/" + args[1]
		if summary {
			path += "?summary=true"
		}
		return requestV3(client, "GET", path, nil)
	case "logs":
		if len(args) < 2 {
			return "Usage: iofog-agent ms logs <id> [--follow] [--tail N] [--since ISO8601] [--until ISO8601] [--timestamps]"
		}
		return handleMSLogs(client, args[1], args[2:])
	case "exec":
		if len(args) < 2 {
			return "Usage: iofog-agent ms exec <id> [-- <command...>]"
		}
		command := make([]string, 0)
		for i := 2; i < len(args); i++ {
			if args[i] == "--" {
				command = append(command, args[i+1:]...)
				break
			}
			return fmt.Sprintf("Error[INVALID_ARGUMENT]: unknown flag %s", args[i])
		}
		payload := map[string]interface{}{
			"command": command,
			"tty":     true,
			"stdin":   true,
			"stdout":  true,
			"stderr":  true,
		}
		result, err := client.RequestV3("POST", "/v3/ms/"+args[1]+"/exec/sessions", payload)
		if err != nil {
			return formatV3RequestError(err)
		}
		sessionID := mapValueAsString(result, "sessionId")
		if sessionID == "<unknown>" {
			return "Error[INTERNAL]: exec session id missing from response"
		}
		return attachExecSessionWS(client, args[1], sessionID)
	case "start":
		if len(args) < 2 {
			return "Usage: iofog-agent ms start <id>"
		}
		return requestV3(client, "POST", "/v3/ms/"+args[1]+"/start", nil)
	case "stop":
		if len(args) < 2 {
			return "Usage: iofog-agent ms stop <id>"
		}
		return requestV3(client, "POST", "/v3/ms/"+args[1]+"/stop", nil)
	case "kill":
		if len(args) < 2 {
			return "Usage: iofog-agent ms kill <id>"
		}
		return requestV3(client, "POST", "/v3/ms/"+args[1]+"/kill", nil)
	case "restart":
		if len(args) < 2 {
			return "Usage: iofog-agent ms restart <id>"
		}
		return requestV3(client, "POST", "/v3/ms/"+args[1]+"/restart", nil)
	case "rm":
		if len(args) < 2 {
			return "Usage: iofog-agent ms rm <id>"
		}
		return requestV3(client, "DELETE", "/v3/ms/"+args[1], nil)
	default:
		return showMSHelpV3()
	}
}

func handleDeployV3(client *Client, args []string) string {
	if len(args) > 0 && isHelpArg(args[0]) {
		return showDeployHelpV3()
	}
	target := "microservices"
	if len(args) > 0 && (args[0] == "registry" || args[0] == "registries") {
		target = "registries"
		args = args[1:]
	}
	mode := "apply"
	fileArgOffset := 0
	if len(args) > 0 && (args[0] == "apply" || args[0] == "validate") {
		mode = args[0]
		fileArgOffset = 1
	}
	if len(args) < fileArgOffset+2 || args[fileArgOffset] != "-f" {
		return showDeployHelpV3()
	}
	manifestPath := args[fileArgOffset+1]
	if target == "microservices" && len(args) > 0 && args[0] != "registry" && args[0] != "registries" {
		if kind, err := detectManifestKind(manifestPath); err == nil && strings.EqualFold(kind, "Registry") {
			target = "registries"
		}
	}
	fields := map[string]string{}
	for i := fileArgOffset + 2; i < len(args); i++ {
		switch args[i] {
		case "--sourceName":
			if i+1 >= len(args) {
				return "Usage: --sourceName <value>"
			}
			fields["sourceName"] = args[i+1]
			i++
		case "--dry-run":
			fields["dryRun"] = "true"
		}
	}
	if mode == "validate" {
		result, err := client.RequestV3MultipartFile("POST", "/v3/deploy/"+target+":validate", "manifest", manifestPath, fields)
		if err != nil {
			return formatV3RequestError(err)
		}
		return formatV3Output("/v3/deploy/"+target+":validate", result)
	}
	if target == "microservices" {
		fields["async"] = "true"
		return handleDeployApplyWithProgress(client, manifestPath, fields)
	}
	result, err := client.RequestV3MultipartFile("POST", "/v3/deploy/"+target+":apply", "manifest", manifestPath, fields)
	if err != nil {
		return formatV3RequestError(err)
	}
	return formatV3Output("/v3/deploy/"+target+":apply", result)
}

func handleRegistryV3(client *Client, args []string) string {
	if len(args) > 0 && isHelpArg(args[0]) {
		return showRegistryHelpV3()
	}
	if len(args) == 0 {
		return requestV3(client, "GET", "/v3/deploy/registries", nil)
	}
	switch args[0] {
	case "ls":
		return requestV3(client, "GET", "/v3/deploy/registries", nil)
	case "inspect":
		if len(args) < 2 {
			return "Usage: iofog-agent registry inspect <id>"
		}
		items, err := client.RequestV3("GET", "/v3/deploy/registries", nil)
		if err != nil {
			return formatV3RequestError(err)
		}
		return formatRegistryInspect(items, args[1])
	case "rm":
		if len(args) < 2 {
			return "Usage: iofog-agent registry rm <id>"
		}
		return requestV3(client, "DELETE", "/v3/deploy/registries/"+args[1], nil)
	default:
		return showRegistryHelpV3()
	}
}

func handleAuthV3(client *Client, args []string) string {
	if len(args) > 0 && isHelpArg(args[0]) {
		return showAuthHelpV3()
	}
	if len(args) == 0 {
		return showAuthHelpV3()
	}
	switch args[0] {
	case "whoami":
		return requestV3(client, "GET", "/v3/auth/whoami", nil)
	case "tokens":
		return requestV3(client, "GET", "/v3/auth/tokens", nil)
	case "revoke":
		if len(args) < 2 {
			return "Usage: iofog-agent auth revoke <jti>"
		}
		return requestV3(client, "POST", "/v3/auth/tokens/revoke", map[string]interface{}{"jti": args[1]})
	default:
		return showAuthHelpV3()
	}
}

func handleImageV3(client *Client, args []string) string {
	if len(args) == 0 || isHelpArg(args[0]) {
		return showImageHelpV3()
	}
	switch args[0] {
	case "ls":
		if len(args) > 1 {
			return "Usage: iofog-agent image ls"
		}
		return requestV3(client, "GET", "/v3/images", nil)
	case "pull":
		if len(args) < 2 {
			return "Usage: iofog-agent image pull <image> [-r|--registry-id <id>] [-p|--platform <platform>]"
		}
		imageRef := strings.TrimSpace(args[1])
		if imageRef == "" {
			return "Error[INVALID_ARGUMENT]: image is required"
		}
		payload := map[string]interface{}{"image": imageRef}
		for i := 2; i < len(args); i++ {
			switch args[i] {
			case "-r", "--registry-id":
				if i+1 >= len(args) {
					return "Error[INVALID_ARGUMENT]: --registry-id requires a positive integer"
				}
				id, err := strconv.Atoi(strings.TrimSpace(args[i+1]))
				if err != nil || id <= 0 {
					return "Error[INVALID_ARGUMENT]: --registry-id requires a positive integer"
				}
				payload["registryId"] = id
				i++
			case "-p", "--platform":
				if i+1 >= len(args) {
					return "Error[INVALID_ARGUMENT]: --platform requires os/arch[/variant]"
				}
				platform := strings.TrimSpace(args[i+1])
				if platform == "" {
					return "Error[INVALID_ARGUMENT]: --platform requires os/arch[/variant]"
				}
				payload["platform"] = platform
				i++
			case "-h", "--help", "-?":
				return "Usage: iofog-agent image pull <image> [-r|--registry-id <id>] [-p|--platform <platform>]"
			default:
				return fmt.Sprintf("Error[INVALID_ARGUMENT]: unknown flag %s", args[i])
			}
		}
		payload["async"] = true
		return handleImagePullWithProgress(client, payload)
	case "load":
		if len(args) < 3 {
			return "Usage: iofog-agent image load -f <path-to-tar-file>"
		}
		path := ""
		for i := 1; i < len(args); i++ {
			switch args[i] {
			case "-f", "--file":
				if i+1 >= len(args) {
					return "Error[INVALID_ARGUMENT]: -f requires path-to-tar-file"
				}
				path = strings.TrimSpace(args[i+1])
				i++
			case "-h", "--help", "-?":
				return "Usage: iofog-agent image load -f <path-to-tar-file>"
			default:
				return fmt.Sprintf("Error[INVALID_ARGUMENT]: unknown flag %s", args[i])
			}
		}
		if path == "" {
			return "Error[INVALID_ARGUMENT]: path is required"
		}
		return requestV3(client, "POST", "/v3/images:load", map[string]interface{}{"path": path})
	case "prune":
		mode, errOut := parseImagePruneModeArgs(args[1:], "Usage: iofog-agent image prune [dangling]")
		if errOut != "" {
			return errOut
		}
		path := "/v3/images:prune"
		if mode != "" {
			path += "?mode=" + mode
		}
		return requestV3(client, "POST", path, nil)
	case "rm":
		if len(args) != 2 {
			return "Usage: iofog-agent image rm <image-id|id-prefix|name[:tag]|digest>"
		}
		selector := strings.TrimSpace(args[1])
		if selector == "" {
			return "Error[INVALID_ARGUMENT]: selector is required"
		}
		return requestV3(client, "POST", "/v3/images:remove", map[string]interface{}{"selector": selector})
	default:
		return showImageHelpV3()
	}
}

func parseImagePruneModeArgs(args []string, usage string) (string, string) {
	mode, errOut := parsePruneModeArgs(args, usage)
	if errOut != "" {
		return "", errOut
	}
	if mode == "" {
		return "", ""
	}
	if mode != "dangling" {
		return "", "Error[INVALID_ARGUMENT]: image prune supports only dangling mode"
	}
	return mode, ""
}

func handleImagePullWithProgress(client *Client, payload map[string]interface{}) string {
	startResult, err := client.RequestV3("POST", "/v3/images:pull", payload)
	if err != nil {
		return formatV3RequestError(err)
	}
	operationID := strings.TrimSpace(mapValueAsString(startResult, "operationId"))
	if operationID == "" || operationID == "<unknown>" {
		return "Error[INTERNAL]: missing image pull operationId in response"
	}
	lastProgress := -1
	for {
		statusResult, err := client.RequestV3("GET", "/v3/images:pull/"+operationID, nil)
		if err != nil {
			return formatV3RequestError(err)
		}
		progress := 0
		switch typed := statusResult["progress"].(type) {
		case float64:
			progress = int(typed)
		case int:
			progress = typed
		}
		if progress < 0 {
			progress = 0
		}
		if progress > 100 {
			progress = 100
		}
		if progress != lastProgress {
			fmt.Printf("\rpulling image... %d%%", progress)
			lastProgress = progress
		}
		status := strings.ToLower(strings.TrimSpace(mapValueAsString(statusResult, "status")))
		switch status {
		case "succeeded":
			fmt.Print("\r")
			return formatImagePullResult(statusResult)
		case "failed":
			fmt.Print("\r")
			errMsg := strings.TrimSpace(mapValueAsString(statusResult, "error"))
			if errMsg == "" || errMsg == "<unknown>" {
				errMsg = "image pull failed"
			}
			return "Error[INTERNAL]: " + errMsg
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func handleDeployApplyWithProgress(client *Client, manifestPath string, fields map[string]string) string {
	startResult, err := client.RequestV3MultipartFile("POST", "/v3/deploy/microservices:apply", "manifest", manifestPath, fields)
	if err != nil {
		return formatV3RequestError(err)
	}
	operationID := strings.TrimSpace(mapValueAsString(startResult, "operationId"))
	if operationID == "" || operationID == "<unknown>" {
		return "Error[INTERNAL]: missing deploy apply operationId in response"
	}
	lastLine := ""
	for {
		statusResult, err := client.RequestV3("GET", "/v3/deploy/microservices:apply/"+operationID, nil)
		if err != nil {
			return formatV3RequestError(err)
		}
		status := strings.ToLower(strings.TrimSpace(mapValueAsString(statusResult, "status")))
		stage := strings.TrimSpace(mapValueAsString(statusResult, "stage"))
		if stage == "<unknown>" {
			stage = ""
		}
		if status == "running" {
			line := formatDeployApplyProgressLine(stage)
			if line != lastLine {
				fmt.Printf("\r%s", line)
				lastLine = line
			}
			time.Sleep(500 * time.Millisecond)
			continue
		}
		fmt.Print("\r")
		if status == "succeeded" {
			return formatDeployApplyResult(statusResult)
		}
		code, message := formatDeployApplyError(statusResult)
		return fmt.Sprintf("Error[%s]: %s", code, message)
	}
}

func formatDeployApplyProgressLine(stage string) string {
	stage = strings.TrimSpace(stage)
	if stage == "" || stage == "<unknown>" {
		return "applying microservice manifest..."
	}
	return fmt.Sprintf("applying microservice manifest... (%s)", stage)
}

func formatDeployApplyError(statusResult map[string]interface{}) (string, string) {
	code := "INTERNAL"
	message := ""
	if rawErr, ok := statusResult["error"].(map[string]interface{}); ok {
		if c := strings.TrimSpace(mapValueAsString(rawErr, "code")); c != "" && c != "<unknown>" {
			code = c
		}
		if m := strings.TrimSpace(mapValueAsString(rawErr, "message")); m != "" && m != "<unknown>" {
			message = m
		}
	}
	if message == "" || message == "<unknown>" {
		message = strings.TrimSpace(mapValueAsString(statusResult, "error"))
	}
	if message == "" || message == "<unknown>" {
		message = "deploy apply failed"
	}
	return code, message
}

func requestV3(client *Client, method, path string, payload interface{}) string {
	result, err := client.RequestV3(method, path, payload)
	if err != nil {
		return formatV3RequestError(err)
	}
	if len(result) == 0 {
		return ""
	}
	if formatted := formatV3Output(path, result); formatted != "" {
		return formatted
	}
	raw, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Sprintf("%v", result)
	}
	return string(raw)
}

func formatV3RequestError(err error) string {
	if apiErr, ok := err.(*V3APIError); ok {
		code := apiErr.Code
		if code == "" {
			code = "HTTP_ERROR"
		}
		return fmt.Sprintf("Error[%s]: %s", code, apiErr.Message)
	}
	return fmt.Sprintf("Error[INTERNAL]: %v", err)
}

func formatV3Output(path string, result map[string]interface{}) string {
	routePath := stripQuery(path)
	switch routePath {
	case "/v3/system/status":
		return formatFlatMapWithOrder(result, statusOutputOrder)
	case "/v3/system/info":
		return formatInfoWithAliasOrder(result)
	case "/v3/system/config":
		return formatConfigPatchResult(result)
	case "/v3/system/controller/cert":
		return "controller certificate updated successfully"
	case "/v3/system/config/switch":
		return formatSwitchResult(result)
	case "/v3/ms":
		return formatMSList(result)
	case "/v3/images":
		return formatImageList(result)
	case "/v3/images:pull":
		return formatImagePullResult(result)
	case "/v3/images:load":
		return formatImageLoadResult(result)
	case "/v3/images:prune", "/v3/system/prune":
		return formatImagePruneResult(result)
	case "/v3/images:remove":
		return formatImageRemoveResult(result)
	case "/v3/deploy/registries":
		return formatRegistryList(result)
	case "/v3/deploy/microservices:validate", "/v3/deploy/registries:validate":
		return formatDeployValidateResult(result)
	case "/v3/deploy/microservices:apply", "/v3/deploy/registries:apply":
		return formatDeployApplyResult(result)
	default:
		if strings.HasPrefix(routePath, "/v3/ms/") {
			if strings.HasSuffix(routePath, "/start") || strings.HasSuffix(routePath, "/stop") ||
				strings.HasSuffix(routePath, "/restart") || strings.HasSuffix(routePath, "/kill") {
				return formatMSLifecycleResult(routePath, result)
			}
			// /v3/ms/{id} is shared by inspect + rm. Only format rm-style payloads.
			if _, ok := result["microserviceUuid"]; ok {
				if status, hasStatus := result["status"]; hasStatus && fmt.Sprintf("%v", status) == "ok" {
					return formatMSLifecycleResult(routePath, result)
				}
			}
		}
		if strings.HasPrefix(routePath, "/v3/deploy/registries/") {
			if status, ok := result["status"]; ok && fmt.Sprintf("%v", status) == "ok" {
				return formatRegistryRemoveResult(result)
			}
		}
		return ""
	}
}

func formatDeployValidateResult(result map[string]interface{}) string {
	if valid, ok := result["valid"].(bool); ok && valid {
		return fmt.Sprintf("manifest is valid (kind=%v name=%v apiVersion=%v)", result["kind"], result["name"], result["apiVersion"])
	}
	return "manifest validation result unavailable"
}

func formatDeployApplyResult(result map[string]interface{}) string {
	if strings.EqualFold(strings.TrimSpace(mapValueAsString(result, "status")), "succeeded") {
		if id := mapValueAsString(result, "deploymentId"); id != "<unknown>" {
			return fmt.Sprintf("microservice manifest applied successfully (deploymentId=%s)", id)
		}
		return "microservice manifest applied successfully"
	}
	if accepted, ok := result["accepted"].(bool); ok && accepted {
		kind := strings.ToLower(strings.TrimSpace(mapValueAsString(result, "kind")))
		switch kind {
		case "registry":
			if reg, ok := result["registry"].(map[string]interface{}); ok {
				return fmt.Sprintf("registry manifest applied successfully (id=%s url=%s)", mapValueAsString(reg, "id"), mapValueAsString(reg, "url"))
			}
			return "registry manifest applied successfully"
		case "microservice":
			if id := mapValueAsString(result, "deploymentId"); id != "<unknown>" {
				return fmt.Sprintf("microservice manifest applied successfully (deploymentId=%s)", id)
			}
			return "microservice manifest applied successfully"
		default:
			if id := mapValueAsString(result, "deploymentId"); id != "<unknown>" {
				return fmt.Sprintf("manifest applied successfully (deploymentId=%s)", id)
			}
			return "manifest applied successfully"
		}
	}
	return "manifest apply result unavailable"
}

func formatMSList(result map[string]interface{}) string {
	rawItems, ok := result["items"].([]interface{})
	if !ok || len(rawItems) == 0 {
		return "No microservices found."
	}
	rows := [][]string{
		{"UUID", "APPLICATIONNAME", "MICROSERVICENAME", "STATE", "CONTAINERID", "IMAGE", "TYPE"},
	}
	for _, raw := range rawItems {
		item, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		rows = append(rows, []string{
			mapValueAsString(item, "uuid"),
			mapValueAsString(item, "application"),
			mapValueAsString(item, "name"),
			mapValueAsString(item, "state"),
			mapValueAsString(item, "containerId"),
			mapValueAsString(item, "image"),
			mapValueAsString(item, "type"),
		})
	}
	return formatAlignedTable(rows)
}

func formatRegistryList(result map[string]interface{}) string {
	rawItems, ok := result["items"].([]interface{})
	if !ok || len(rawItems) == 0 {
		return "No registries found."
	}
	rows := [][]string{
		{"ID", "URL", "PUBLIC", "USERNAME", "EMAIL"},
	}
	for _, raw := range rawItems {
		item, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		rows = append(rows, []string{
			mapValueAsString(item, "id"),
			mapValueAsString(item, "url"),
			mapValueAsString(item, "isPublic"),
			mapValueAsString(item, "userName"),
			mapValueAsString(item, "userEmail"),
		})
	}
	return formatAlignedTable(rows)
}

func formatImageList(result map[string]interface{}) string {
	rawItems, ok := result["items"].([]interface{})
	if !ok || len(rawItems) == 0 {
		return "No images found."
	}
	rows := [][]string{
		{"REPOSITORY", "TAG", "IMAGE ID", "CREATED", "SIZE"},
	}
	for _, raw := range rawItems {
		item, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		rows = append(rows, []string{
			valueOrDefault(mapValueAsString(item, "repository"), "<none>"),
			valueOrDefault(mapValueAsString(item, "tag"), "<none>"),
			valueOrDefault(mapValueAsString(item, "shortId"), "<none>"),
			humanizeCreated(mapValueAsString(item, "createdAt")),
			valueOrDefault(mapValueAsString(item, "sizeHuman"), "0 B"),
		})
	}
	return formatAlignedTable(rows)
}

func formatImagePullResult(result map[string]interface{}) string {
	return fmt.Sprintf(
		"image pulled successfully: %s (engine=%s, platform=%s)",
		mapValueAsString(result, "resolvedImage"),
		valueOrDefault(mapValueAsString(result, "engine"), "<unknown>"),
		valueOrDefault(mapValueAsString(result, "platform"), "<none>"),
	)
}

func formatImageRemoveResult(result map[string]interface{}) string {
	return fmt.Sprintf(
		"image removed successfully: %s (engine=%s)",
		valueOrDefault(mapValueAsString(result, "removed"), valueOrDefault(mapValueAsString(result, "selector"), "<unknown>")),
		valueOrDefault(mapValueAsString(result, "engine"), "<unknown>"),
	)
}

func formatImageLoadResult(result map[string]interface{}) string {
	return fmt.Sprintf(
		"image archive loaded successfully: %s image imported (engine=%s)",
		mapValueAsString(result, "count"),
		valueOrDefault(mapValueAsString(result, "engine"), "<unknown>"),
	)
}

func formatImagePruneResult(result map[string]interface{}) string {
	mode := strings.ToLower(strings.TrimSpace(mapValueAsString(result, "mode")))
	engineName := valueOrDefault(mapValueAsString(result, "engine"), "<unknown>")
	switch mode {
	case "containers":
		return fmt.Sprintf(
			"pruned containers: deleted=%s (engine=%s)",
			mapValueAsString(result, "deletedCount"),
			engineName,
		)
	case "volumes":
		return fmt.Sprintf(
			"pruned volumes: deleted=%s reclaimed=%s (engine=%s)",
			mapValueAsString(result, "deletedCount"),
			valueOrDefault(mapValueAsString(result, "spaceReclaimedHuman"), valueOrDefault(mapValueAsString(result, "spaceReclaimedBytes"), "0 B")),
			engineName,
		)
	case "all":
		return fmt.Sprintf(
			"pruned all: containers=%s volumes=%s images=%s reclaimed=%s (engine=%s)",
			mapValueAsString(result, "containersDeletedCount"),
			mapValueAsString(result, "volumesDeletedCount"),
			mapValueAsString(result, "imagesDeletedCount"),
			valueOrDefault(mapValueAsString(result, "spaceReclaimedHuman"), valueOrDefault(mapValueAsString(result, "spaceReclaimedBytes"), "0 B")),
			engineName,
		)
	}
	return fmt.Sprintf(
		"pruned dangling images: deleted=%s reclaimed=%s (engine=%s)",
		mapValueAsString(result, "deletedCount"),
		valueOrDefault(mapValueAsString(result, "spaceReclaimedHuman"), valueOrDefault(mapValueAsString(result, "spaceReclaimedBytes"), "0 B")),
		engineName,
	)
}

func formatRegistryInspect(result map[string]interface{}, id string) string {
	rawItems, ok := result["items"].([]interface{})
	if !ok {
		return "No registries found."
	}
	for _, raw := range rawItems {
		item, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if mapValueAsString(item, "id") == id {
			return strings.Join([]string{
				fmt.Sprintf("ID: %s", mapValueAsString(item, "id")),
				fmt.Sprintf("URL: %s", mapValueAsString(item, "url")),
				fmt.Sprintf("PUBLIC: %s", mapValueAsString(item, "isPublic")),
				fmt.Sprintf("USERNAME: %s", mapValueAsString(item, "userName")),
				fmt.Sprintf("EMAIL: %s", mapValueAsString(item, "userEmail")),
			}, "\n")
		}
	}
	return fmt.Sprintf("Error[NOT_FOUND]: registry %s not found", id)
}

func formatRegistryRemoveResult(result map[string]interface{}) string {
	if id := mapValueAsString(result, "id"); id != "<unknown>" {
		return fmt.Sprintf("registry removed successfully (id=%s)", id)
	}
	return "registry removed successfully"
}

func formatMSLifecycleResult(path string, result map[string]interface{}) string {
	operation := "operation"
	switch {
	case strings.HasSuffix(path, "/start"):
		operation = "start"
	case strings.HasSuffix(path, "/stop"):
		operation = "stop"
	case strings.HasSuffix(path, "/restart"):
		operation = "restart"
	case strings.HasSuffix(path, "/kill"):
		operation = "kill"
	default:
		operation = "rm"
	}
	uuid := mapValueAsString(result, "microserviceUuid")
	msg := fmt.Sprintf("microservice %s completed successfully", operation)
	if uuid != "<unknown>" {
		msg = fmt.Sprintf("microservice %s completed successfully (uuid=%s)", operation, uuid)
	}
	if warning := strings.TrimSpace(mapValueAsString(result, "warning")); warning != "" && warning != "<unknown>" {
		msg += "\nwarning: " + warning
	}
	return msg
}

func formatAlignedTable(rows [][]string) string {
	if len(rows) == 0 {
		return ""
	}
	var b strings.Builder
	w := tabwriter.NewWriter(&b, 0, 8, 2, ' ', 0)
	for _, row := range rows {
		if len(row) == 0 {
			continue
		}
		_, _ = fmt.Fprintln(w, strings.Join(row, "\t"))
	}
	_ = w.Flush()
	return strings.TrimRight(b.String(), "\n")
}

func stripQuery(path string) string {
	if idx := strings.Index(path, "?"); idx >= 0 {
		return path[:idx]
	}
	return path
}

func valueOrDefault(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "<unknown>" {
		return fallback
	}
	return value
}

func humanizeCreated(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "<unknown>" {
		return "<unknown>"
	}
	ts, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return raw
	}
	d := time.Since(ts)
	if d < time.Minute {
		return "just now"
	}
	if d < time.Hour {
		return fmt.Sprintf("%d minutes ago", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%d hours ago", int(d.Hours()))
	}
	return fmt.Sprintf("%d days ago", int(d.Hours()/24))
}

var statusOutputOrder = []string{
	"connectionToController",
	"cpuUsage",
	"diskUsage",
	"iofogDaemon",
	"memoryUsage",
	"runningMicroservices",
	"systemAvailableDisk",
	"systemAvailableMemory",
	"systemTime",
	"systemTotalCpu",
	"availableNetworkInterfaces",
}

var infoOutputOrder = []string{
	"iofogUuid",
	"namespace",
	"networkInterface",
	"ipAddress",
	"controllerUrl",
	"controllerCert",
	"secureMode",
	"containerEngine",
	"dockerUrl",
	"arch",
	"availableDiskThreshold",
	"changeFrequency",
	"statusFrequency",
	"cpuLimit",
	"memoryLimit",
	"diskLimit",
	"diskDirectory",
	"logLimit",
	"logFileDirectory",
	"logFilesCount",
	"logLevel",
	"upgradeScanFrequency",
	"deviceScanFrequency",
	"pruningFrequency",
	"edgeGuardFrequency",
	"gpsCoordinates",
	"gpsDevice",
	"gpsScanFrequency",
	"gpsMode",
	"watchdogEnabled",
	"developerMode",
	"timeZone",
}

var infoAliasToCanonical = map[string]string{
	"arch":                 "fogType",
	"changeFrequency":      "changeUpdateFrequency",
	"statusFrequency":      "statusUpdateFrequency",
	"cpuLimit":             "cpuUsageLimit",
	"memoryLimit":          "memoryRamLimit",
	"diskLimit":            "diskUsageLimit",
	"logLimit":             "logDiskLimit",
	"logLevel":             "logFilesLevel",
	"upgradeScanFrequency": "readyToUpgradeScanFrequency",
	"deviceScanFrequency":  "scanDevicesFrequency",
	"pruningFrequency":     "dockerPruningFrequency",
}

func formatFlatMapWithOrder(result map[string]interface{}, preferred []string) string {
	if len(result) == 0 {
		return ""
	}
	for _, value := range result {
		switch value.(type) {
		case map[string]interface{}, []interface{}:
			return ""
		}
	}
	seen := make(map[string]bool, len(result))
	var b strings.Builder
	for _, key := range preferred {
		value, ok := result[key]
		if !ok {
			continue
		}
		fmt.Fprintf(&b, "%s: %v\n", key, value)
		seen[key] = true
	}
	remaining := make([]string, 0, len(result))
	for key := range result {
		if !seen[key] {
			remaining = append(remaining, key)
		}
	}
	sort.Strings(remaining)
	for _, key := range remaining {
		fmt.Fprintf(&b, "%s: %v\n", key, result[key])
	}
	return strings.TrimRight(b.String(), "\n")
}

func formatInfoWithAliasOrder(result map[string]interface{}) string {
	if len(result) == 0 {
		return ""
	}
	for _, value := range result {
		switch value.(type) {
		case map[string]interface{}, []interface{}:
			return ""
		}
	}

	seenCanonical := make(map[string]bool, len(result))
	var b strings.Builder
	for _, alias := range infoOutputOrder {
		canonical := alias
		if mapped, ok := infoAliasToCanonical[alias]; ok {
			canonical = mapped
		}
		value, ok := result[canonical]
		if !ok {
			continue
		}
		fmt.Fprintf(&b, "%s: %v\n", alias, value)
		seenCanonical[canonical] = true
	}

	remainingAliases := make([]string, 0, len(result))
	for canonical := range result {
		if seenCanonical[canonical] {
			continue
		}
		remainingAliases = append(remainingAliases, canonical)
	}
	sort.Strings(remainingAliases)
	for _, key := range remainingAliases {
		fmt.Fprintf(&b, "%s: %v\n", key, result[key])
	}
	return strings.TrimRight(b.String(), "\n")
}

func isHelpArg(arg string) bool {
	return arg == "--help" || arg == "-h" || arg == "-?"
}

func parseConfigSetArgs(args []string) (map[string]interface{}, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("config requires at least one key/value pair")
	}
	setMap := make(map[string]interface{})

	if len(args)%2 != 0 {
		return nil, fmt.Errorf("config arguments must be key/value pairs")
	}

	for i := 0; i < len(args); i += 2 {
		rawKey := args[i]
		value := args[i+1]
		normalized, rule, ok := lookupConfigKeyRule(rawKey)
		if !ok {
			return nil, fmt.Errorf("unsupported config key %q", rawKey)
		}
		normalizedValue, err := validateAndNormalizeConfigValue(rule, value)
		if err != nil {
			return nil, fmt.Errorf("%s: %v", normalized, err)
		}
		setMap[normalized] = normalizedValue
	}
	return setMap, nil
}

func showConfigHelpV3() string {
	return "Usage:\n" +
		"  iofog-agent config <key> <value> [<key> <value> ...]\n" +
		"  iofog-agent config -n <iface> -a <controllerUrl> ...\n\n" +
		"Examples:\n" +
		"  iofog-agent config networkInterface lima0\n" +
		"  iofog-agent config -n lima0 -a http://192.168.1.8:51121/api/v3\n" +
		"  iofog-agent config -cf 10 -sf 10 -ll DEBUG\n\n" +
		showConfigSetHelpV3()
}

func showConfigSetHelpV3() string {
	var b strings.Builder
	b.WriteString("Supported config keys (canonical | aliases):\n")
	keys := sortedConfigRuleKeys()
	for _, key := range keys {
		rule := configKeyRules[key]
		fmt.Fprintf(&b, "  %s", key)
		if len(rule.Aliases) > 0 {
			fmt.Fprintf(&b, " | %s", strings.Join(rule.Aliases, ", "))
		}
		if rule.Help != "" {
			fmt.Fprintf(&b, "  (%s)", rule.Help)
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

type configValueType string

const (
	configValueString configValueType = "string"
	configValueInt    configValueType = "int"
	configValueFloat  configValueType = "float"
	configValueBool   configValueType = "bool"
)

type configKeyRule struct {
	Key     string
	Aliases []string
	Type    configValueType
	Enums   []string
	Min     *float64
	Max     *float64
	Help    string
}

var configKeyRules = map[string]configKeyRule{
	"controllerUrl":          {Key: "controllerUrl", Aliases: []string{"a", "-a"}, Type: configValueString, Help: "controller URL"},
	"controllerCert":         {Key: "controllerCert", Aliases: []string{"ac", "-ac"}, Type: configValueString, Help: "controller CA certificate file path"},
	"containerEngine":        {Key: "containerEngine", Aliases: []string{"ce", "-ce"}, Type: configValueString, Enums: []string{"docker", "podman", "iofog"}, Help: "container engine"},
	"dockerUrl":              {Key: "dockerUrl", Aliases: []string{"c", "-c"}, Type: configValueString, Help: "runtime socket URL"},
	"networkInterface":       {Key: "networkInterface", Aliases: []string{"n", "-n"}, Type: configValueString, Help: "network interface"},
	"diskLimitGiB":           {Key: "diskLimitGiB", Aliases: []string{"d", "-d"}, Type: configValueFloat, Help: "disk usage limit (GiB)"},
	"diskDirectory":          {Key: "diskDirectory", Aliases: []string{"dl", "-dl"}, Type: configValueString, Help: "disk directory"},
	"memoryLimitMiB":         {Key: "memoryLimitMiB", Aliases: []string{"m", "-m"}, Type: configValueFloat, Help: "memory limit (MiB)"},
	"cpuLimitPercent":        {Key: "cpuLimitPercent", Aliases: []string{"p", "-p"}, Type: configValueFloat, Help: "CPU limit (%)"},
	"logDiskLimitGiB":        {Key: "logDiskLimitGiB", Aliases: []string{"l", "-l"}, Type: configValueFloat, Help: "log disk limit (GiB)"},
	"logDiskDirectory":       {Key: "logDiskDirectory", Aliases: []string{"ld", "-ld"}, Type: configValueString, Help: "log directory"},
	"logFileCount":           {Key: "logFileCount", Aliases: []string{"lc", "-lc"}, Type: configValueInt, Help: "log file count"},
	"logLevel":               {Key: "logLevel", Aliases: []string{"ll", "-ll"}, Type: configValueString, Enums: []string{"DEBUG", "INFO", "WARN", "ERROR"}, Help: "log level"},
	"statusFrequencySeconds": {Key: "statusFrequencySeconds", Aliases: []string{"sf", "-sf"}, Type: configValueInt, Help: "status frequency (seconds)"},
	"changeFrequencySeconds": {Key: "changeFrequencySeconds", Aliases: []string{"cf", "-cf"}, Type: configValueInt, Help: "change polling frequency (seconds)"},
	"deviceScanFrequency":    {Key: "deviceScanFrequency", Aliases: []string{"sd", "-sd"}, Type: configValueInt, Help: "device scan frequency (seconds)"},
	"watchdogEnabled":        {Key: "watchdogEnabled", Aliases: []string{"idc", "-idc"}, Type: configValueBool, Help: "watchdog enable"},
	"edgeGuardFrequency":     {Key: "edgeGuardFrequency", Aliases: []string{"egf", "-egf"}, Type: configValueInt, Help: "edge guard frequency"},
	"gpsMode":                {Key: "gpsMode", Aliases: []string{"gps", "-gps"}, Type: configValueString, Enums: []string{"auto", "dynamic", "manual", "off"}, Help: "GPS mode"},
	"gpsCoordinates":         {Key: "gpsCoordinates", Aliases: []string{"gpsc", "-gpsc"}, Type: configValueString, Help: "GPS coordinates lat,lon"},
	"gpsDevice":              {Key: "gpsDevice", Aliases: []string{"gpsd", "-gpsd"}, Type: configValueString, Help: "GPS device"},
	"gpsScanFrequency":       {Key: "gpsScanFrequency", Aliases: []string{"gpsf", "-gpsf"}, Type: configValueInt, Help: "GPS scan frequency"},
	"arch":                   {Key: "arch", Aliases: []string{"ft", "-ft"}, Type: configValueString, Help: "fog type/arch"},
	"secureMode":             {Key: "secureMode", Aliases: []string{"sec", "-sec"}, Type: configValueBool, Help: "secure mode"},
	"dockerPruningFrequency": {Key: "dockerPruningFrequency", Aliases: []string{"pf", "-pf"}, Type: configValueInt, Help: "prune frequency"},
	"availableDiskThreshold": {Key: "availableDiskThreshold", Aliases: []string{"dt", "-dt"}, Type: configValueInt, Help: "available disk threshold"},
	"upgradeScanFrequency":   {Key: "upgradeScanFrequency", Aliases: []string{"uf", "-uf"}, Type: configValueInt, Help: "upgrade scan frequency"},
	"devMode":                {Key: "devMode", Aliases: []string{"dev", "-dev"}, Type: configValueBool, Help: "developer mode"},
	"timezone":               {Key: "timezone", Aliases: []string{"tz", "-tz", "timeZone"}, Type: configValueString, Help: "timezone"},
}

func lookupConfigKeyRule(raw string) (string, configKeyRule, bool) {
	key := strings.TrimSpace(raw)
	key = strings.TrimPrefix(key, "--")
	key = strings.TrimPrefix(key, "-")
	for canonical, rule := range configKeyRules {
		if strings.EqualFold(canonical, key) || strings.EqualFold(rule.Key, key) {
			return canonical, rule, true
		}
		for _, alias := range rule.Aliases {
			a := strings.TrimPrefix(strings.TrimPrefix(alias, "--"), "-")
			if strings.EqualFold(a, key) {
				return canonical, rule, true
			}
		}
	}
	return "", configKeyRule{}, false
}

func validateAndNormalizeConfigValue(rule configKeyRule, value string) (interface{}, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, fmt.Errorf("value cannot be empty")
	}
	if rule.Key == "controllerCert" {
		data, err := os.ReadFile(value) // #nosec G304 -- user-supplied path intentionally validated for local cert file
		if err != nil {
			return nil, fmt.Errorf("must be a readable PEM certificate file path")
		}
		block, _ := pem.Decode(data)
		if block == nil || block.Type != "CERTIFICATE" {
			return nil, fmt.Errorf("must be a readable PEM certificate file path")
		}
		if _, err := x509.ParseCertificate(block.Bytes); err != nil {
			return nil, fmt.Errorf("must be a readable PEM certificate file path")
		}
		return value, nil
	}
	switch rule.Type {
	case configValueInt:
		i, err := strconv.Atoi(value)
		if err != nil {
			return nil, fmt.Errorf("must be an integer")
		}
		return i, nil
	case configValueFloat:
		f, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return nil, fmt.Errorf("must be a number")
		}
		return f, nil
	case configValueBool:
		switch strings.ToLower(value) {
		case "true", "1", "on", "yes":
			return true, nil
		case "false", "0", "off", "no":
			return false, nil
		default:
			return nil, fmt.Errorf("must be one of true|false|on|off|1|0")
		}
	default:
		if len(rule.Enums) > 0 {
			for _, allowed := range rule.Enums {
				if strings.EqualFold(allowed, value) {
					return value, nil
				}
			}
			return nil, fmt.Errorf("must be one of %s", strings.Join(rule.Enums, "|"))
		}
		return value, nil
	}
}

func sortedConfigRuleKeys() []string {
	keys := make([]string, 0, len(configKeyRules))
	for key := range configKeyRules {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func formatConfigPatchResult(result map[string]interface{}) string {
	if len(result) == 0 {
		return ""
	}
	status, _ := result["status"].(string)
	errorMap, _ := result["errorMap"].(map[string]interface{})
	if len(errorMap) == 0 {
		if status == "" {
			status = "ok"
		}
		return fmt.Sprintf("config update: %s (all requested changes accepted)", status)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "config update: %s\n", status)
	fmt.Fprintln(&b, "rejected keys:")
	keys := make([]string, 0, len(errorMap))
	for k := range errorMap {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(&b, "  - %s: %v\n", k, errorMap[k])
	}
	return strings.TrimRight(b.String(), "\n")
}

func formatConfigMutationOutput(setMap, before, patchResult map[string]interface{}) string {
	if len(setMap) == 0 {
		return "config update: no changes requested"
	}
	errorMap, _ := patchResult["errorMap"].(map[string]interface{})
	status, _ := patchResult["status"].(string)
	if status == "" {
		status = "ok"
	}
	var accepted []string
	var rejected []string
	keys := make([]string, 0, len(setMap))
	for k := range setMap {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if _, failed := errorMap[key]; failed {
			rejected = append(rejected, fmt.Sprintf("%s (%v)", key, errorMap[key]))
			continue
		}
		oldVal := "<unknown>"
		if before != nil {
			if v, ok := before[key]; ok {
				oldVal = fmt.Sprintf("%v", v)
			}
		}
		accepted = append(accepted, fmt.Sprintf("%s: %s -> %v", key, oldVal, setMap[key]))
	}
	var b strings.Builder
	fmt.Fprintf(&b, "config update: %s\n", status)
	if len(accepted) > 0 {
		fmt.Fprintln(&b, "accepted:")
		for _, line := range accepted {
			fmt.Fprintf(&b, "  - %s\n", line)
		}
	}
	if len(rejected) > 0 {
		fmt.Fprintln(&b, "rejected:")
		for _, line := range rejected {
			fmt.Fprintf(&b, "  - %s\n", line)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func formatSwitchResult(result map[string]interface{}) string {
	oldProfile := fmt.Sprintf("%v", result["oldProfile"])
	profile := fmt.Sprintf("%v", result["profile"])
	if strings.TrimSpace(profile) == "" {
		return "configuration profile switched successfully"
	}
	if strings.TrimSpace(oldProfile) != "" {
		return fmt.Sprintf("configuration profile switched: %s -> %s", oldProfile, profile)
	}
	return fmt.Sprintf("configuration profile switched to %s", profile)
}

func formatVersionOutput(cliVersion, cliBuildTime, cliGitCommit string, daemonVersion map[string]interface{}, daemonErr error) string {
	var b strings.Builder
	fmt.Fprintf(&b, "cli.version: %s\n", cliVersion)
	fmt.Fprintf(&b, "cli.buildTime: %s\n", cliBuildTime)
	fmt.Fprintf(&b, "cli.gitCommit: %s\n", cliGitCommit)

	if daemonErr != nil || len(daemonVersion) == 0 {
		fmt.Fprint(&b, "daemon: unavailable")
		return b.String()
	}

	fmt.Fprintf(&b, "daemon.version: %s\n", mapValueAsString(daemonVersion, "version"))
	fmt.Fprintf(&b, "daemon.buildTime: %s\n", mapValueAsString(daemonVersion, "buildTime"))
	fmt.Fprintf(&b, "daemon.gitCommit: %s\n", mapValueAsString(daemonVersion, "gitCommit"))
	fmt.Fprintf(&b, "daemon.flavor: %s\n", mapValueAsString(daemonVersion, "flavor"))

	allowed := mapValueAsString(daemonVersion, "allowedContainerEngine")
	if allowed == "<unknown>" {
		allowed = mapValueAsString(daemonVersion, "allowedEngines")
	}
	fmt.Fprintf(&b, "daemon.allowedContainerEngine: %s", allowed)
	return b.String()
}

func mapValueAsString(input map[string]interface{}, key string) string {
	value, ok := input[key]
	if !ok || value == nil {
		return "<unknown>"
	}
	switch typed := value.(type) {
	case string:
		if strings.TrimSpace(typed) == "" {
			return "<unknown>"
		}
		return typed
	case []string:
		if len(typed) == 0 {
			return "<unknown>"
		}
		return strings.Join(typed, ",")
	case []interface{}:
		if len(typed) == 0 {
			return "<unknown>"
		}
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			parts = append(parts, fmt.Sprintf("%v", item))
		}
		return strings.Join(parts, ",")
	default:
		return fmt.Sprintf("%v", typed)
	}
}

func mapValueAsRawString(input map[string]interface{}, key string) string {
	if input == nil {
		return ""
	}
	value, ok := input[key]
	if !ok || value == nil {
		return ""
	}
	if typed, ok := value.(string); ok {
		return typed
	}
	return fmt.Sprintf("%v", value)
}

func showMSHelpV3() string {
	return "Usage:\n" +
		"  iofog-agent ms ps [--source managed|local|all]\n" +
		"  iofog-agent ms inspect <id> [--summary]\n" +
		"  iofog-agent ms logs <id> [--follow] [--tail N] [--since ISO8601] [--until ISO8601] [--timestamps]\n" +
		"  iofog-agent ms exec <id> [-- <command...>]\n" +
		"  iofog-agent ms start|stop|restart|kill|rm <id>\n\n" +
		"ID selectors: <uuid>, <container-id-prefix>, <application>.<name> (example: local.<name>)"
}

func showDeployHelpV3() string {
	return "Usage:\n" +
		"  iofog-agent deploy -f <manifest.yaml>\n" +
		"  iofog-agent deploy apply -f <manifest.yaml>\n" +
		"  iofog-agent deploy validate -f <manifest.yaml>\n\n" +
		"  optional fields: --sourceName <name> --dry-run\n\n" +
		"  iofog-agent deploy registry apply -f <registry.yaml>\n" +
		"  iofog-agent deploy registry validate -f <registry.yaml>\n\n" +
		"Deploys or validates local microservice/registry manifests via LocalAPI v3."
}

func showRegistryHelpV3() string {
	return "Usage:\n" +
		"  iofog-agent registry ls\n" +
		"  iofog-agent registry inspect <id>\n" +
		"  iofog-agent registry rm <id>"
}

func showAuthHelpV3() string {
	return "Usage:\n" +
		"  iofog-agent auth whoami\n" +
		"  iofog-agent auth tokens\n" +
		"  iofog-agent auth revoke <jti>"
}

func showImageHelpV3() string {
	return "Usage:\n" +
		"  iofog-agent image ls\n" +
		"  iofog-agent image pull <image> [-r|--registry-id <id>] [-p|--platform <platform>]\n" +
		"  iofog-agent image load -f <path-to-tar-file>\n" +
		"  iofog-agent image prune\n" +
		"  iofog-agent image rm <image-id|id-prefix|name[:tag]|digest>"
}

func handleMSLogs(client *Client, id string, args []string) string {
	follow := false
	tail := "100"
	since := ""
	until := ""
	timestamps := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--follow", "-f":
			follow = true
		case "--tail":
			if i+1 >= len(args) {
				return "Error[INVALID_ARGUMENT]: --tail requires a number"
			}
			tail = strings.TrimSpace(args[i+1])
			i++
		case "--since":
			if i+1 >= len(args) {
				return "Error[INVALID_ARGUMENT]: --since requires an ISO8601 timestamp"
			}
			since = strings.TrimSpace(args[i+1])
			i++
		case "--until":
			if i+1 >= len(args) {
				return "Error[INVALID_ARGUMENT]: --until requires an ISO8601 timestamp"
			}
			until = strings.TrimSpace(args[i+1])
			i++
		case "--timestamps":
			timestamps = true
		case "--help", "-h", "-?":
			return "Usage: iofog-agent ms logs <id> [--follow] [--tail N] [--since ISO8601] [--until ISO8601] [--timestamps]"
		default:
			return fmt.Sprintf("Error[INVALID_ARGUMENT]: unknown flag %s", args[i])
		}
	}

	if follow {
		return streamLogsWS(client, id, timestamps)
	}

	path := "/v3/ms/" + id + "/logs?tail=" + tail
	if since != "" {
		path += "&since=" + since
	}
	if until != "" {
		path += "&until=" + until
	}
	result, err := client.RequestV3("GET", path, nil)
	if err != nil {
		return formatV3RequestError(err)
	}
	rawEntries, _ := result["entries"].([]interface{})
	var b strings.Builder
	for _, raw := range rawEntries {
		entry, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		line := mapValueAsRawString(entry, "line")
		if timestamps {
			ts := mapValueAsString(entry, "ts")
			b.WriteString(ts + " ")
		}
		b.WriteString(line)
		if !strings.HasSuffix(line, "\n") {
			b.WriteString("\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func streamLogsWS(client *Client, id string, timestamps bool) string {
	conn, err := dialWS(client, "/v3/ms/"+id+"/logs:stream")
	if err != nil {
		return fmt.Sprintf("Error[INTERNAL]: %v", err)
	}
	defer conn.Close()
	for {
		var event map[string]interface{}
		if err := conn.ReadJSON(&event); err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				return ""
			}
			return ""
		}
		line := mapValueAsRawString(event, "line")
		if timestamps {
			ts := mapValueAsString(event, "ts")
			fmt.Printf("%s %s\n", ts, line)
		} else {
			fmt.Println(line)
		}
	}
}

func attachExecSessionWS(client *Client, selector, sessionID string) string {
	conn, err := dialWS(client, "/v3/ms/"+selector+"/exec/sessions/"+sessionID+":attach")
	if err != nil {
		return fmt.Sprintf("Error[INTERNAL]: %v", err)
	}
	defer conn.Close()

	done := make(chan struct{})
	var doneOnce sync.Once
	exitCode := 0
	resizeDone := make(chan struct{})
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGWINCH)
	if term.IsTerminal(int(os.Stdin.Fd())) {
		if oldState, rawErr := term.MakeRaw(int(os.Stdin.Fd())); rawErr == nil {
			defer func() {
				_ = term.Restore(int(os.Stdin.Fd()), oldState)
			}()
		}
	}
	sendResize := func() {
		cols, rows, err := terminalSize()
		if err != nil {
			return
		}
		_ = conn.WriteJSON(map[string]interface{}{
			"type": "resize",
			"cols": cols,
			"rows": rows,
		})
	}
	sendResize()
	go func() {
		defer close(resizeDone)
		for range sigCh {
			sendResize()
		}
	}()
	go func() {
		buf := make([]byte, 1024)
		for {
			n, readErr := os.Stdin.Read(buf)
			data := buf[:0]
			if n > 0 {
				data = buf[:n]
			}
			if len(data) > 0 {
				_ = conn.WriteMessage(websocket.BinaryMessage, data)
			}
			if readErr != nil {
				_ = conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
				doneOnce.Do(func() { close(done) })
				return
			}
		}
	}()

	for {
		select {
		case <-done:
			signal.Stop(sigCh)
			close(sigCh)
			<-resizeDone
			if exitCode != 0 {
				return fmt.Sprintf("__EXIT_CODE__=%d", exitCode)
			}
			return ""
		default:
		}
		var event map[string]interface{}
		if err := conn.ReadJSON(&event); err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				signal.Stop(sigCh)
				close(sigCh)
				<-resizeDone
				if exitCode != 0 {
					return fmt.Sprintf("__EXIT_CODE__=%d", exitCode)
				}
				return ""
			}
			signal.Stop(sigCh)
			close(sigCh)
			<-resizeDone
			if exitCode != 0 {
				return fmt.Sprintf("__EXIT_CODE__=%d", exitCode)
			}
			return ""
		}
		stream := mapValueAsString(event, "stream")
		line := mapValueAsRawString(event, "line")
		if stream == "control" {
			if rawCode, ok := event["exitCode"]; ok {
				switch typed := rawCode.(type) {
				case float64:
					exitCode = int(typed)
				case int:
					exitCode = typed
				case string:
					if parsed, parseErr := strconv.Atoi(strings.TrimSpace(typed)); parseErr == nil {
						exitCode = parsed
					}
				}
			}
			continue
		}
		if stream == "stderr" {
			_, _ = io.WriteString(os.Stderr, line)
		} else if stream == "stdout" {
			_, _ = io.WriteString(os.Stdout, line)
		}
	}
}

func terminalSize() (uint32, uint32, error) {
	ws, err := os.Stdout.Stat()
	if err != nil {
		return 0, 0, err
	}
	if (ws.Mode() & os.ModeCharDevice) == 0 {
		return 0, 0, fmt.Errorf("stdout is not terminal")
	}
	cols, rows, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		return 0, 0, err
	}
	return uint32(cols), uint32(rows), nil
}

func dialWS(client *Client, path string) (*websocket.Conn, error) {
	wsURL := "wss://localhost:54321" + path
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true, // #nosec G402 local daemon endpoint
		},
	}
	header := map[string][]string{"Authorization": {"Bearer " + strings.TrimSpace(client.token)}}
	conn, _, err := dialer.Dial(wsURL, header)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

func detectManifestKind(path string) (string, error) {
	raw, err := os.ReadFile(path) // #nosec G304 CLI manifest path provided by caller
	if err != nil {
		return "", err
	}
	var doc struct {
		Kind string `yaml:"kind"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return "", err
	}
	return strings.TrimSpace(doc.Kind), nil
}
