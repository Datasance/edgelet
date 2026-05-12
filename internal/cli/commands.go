package cli

import (
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"
	"sort"
	"strconv"
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
		"system":      true,
		"auth":        true,
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
		return handleDeprovisionV3(client)
	case "config":
		return handleConfigV3(client, args)
	case "version", "--version", "-v":
		return handleVersion(client)
	case "help", "--help", "-h", "-?":
		return showHelp()
	case "prune":
		return requestV3(client, "POST", "/v3/system/prune", nil)
	case "cert":
		return handleCert(client, args)
	case "switch":
		return handleSwitch(client, args)
	case "ms":
		return handleMicroserviceV3(client, args[1:])
	case "deploy":
		return handleDeployV3(client, args[1:])
	case "system":
		return handleSystemV3(client, args[1:])
	case "auth":
		return handleAuthV3(client, args[1:])
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

func handleDeprovisionV3(client *Client) string {
	_, err := client.RequestV3("DELETE", "/v3/system/provision", nil)
	if err != nil {
		return formatV3RequestError(err)
	}
	return "agent deprovisioned successfully; started cleanup of all local microservices"
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
		"  status | info | provision <key> | deprovision | prune\n" +
		"  config <key> <value> [<key> <value> ...]\n" +
		"  config -n <iface> -a <controllerUrl> ...\n" +
		"  cert <base64-or-pem-certificate>\n" +
		"  switch <dev|prod|def>\n" +
		"  ms ps\n" +
		"  ms inspect <id>\n" +
		"  ms logs <id>\n" +
		"  ms exec <id> -- <command...>\n" +
		"  ms start|stop|kill|rm <id>\n" +
		"  deploy -f <manifest.yaml>\n" +
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
		return "Usage: iofog-agent system <status|info|version|reload|prune>"
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
		return requestV3(client, "POST", "/v3/system/prune", nil)
	default:
		return "Usage: iofog-agent system <status|info|version|reload|prune>"
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
		return requestV3(client, "GET", "/v3/ms", nil)
	case "inspect":
		if len(args) < 2 {
			return "Usage: iofog-agent ms inspect <id>"
		}
		return requestV3(client, "GET", "/v3/ms/"+args[1], nil)
	case "logs":
		if len(args) < 2 {
			return "Usage: iofog-agent ms logs <id>"
		}
		return requestV3(client, "GET", "/v3/ms/"+args[1]+"/logs", nil)
	case "exec":
		if len(args) < 4 || args[2] != "--" {
			return "Usage: iofog-agent ms exec <id> -- <command...>"
		}
		payload := map[string]interface{}{
			"command": args[3:],
			"tty":     true,
			"stdin":   true,
			"stdout":  true,
			"stderr":  true,
		}
		return requestV3(client, "POST", "/v3/ms/"+args[1]+"/exec/sessions", payload)
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
	data, err := os.ReadFile(manifestPath) // #nosec G304 -- user-provided local file path expected for CLI manifests
	if err != nil {
		return fmt.Sprintf("Error reading manifest file: %v", err)
	}
	payload := map[string]interface{}{
		"manifest": string(data),
	}
	if mode == "validate" {
		return requestV3(client, "POST", "/v3/deploy/"+target+":validate", payload)
	}
	return requestV3(client, "POST", "/v3/deploy/"+target+":apply", payload)
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
	switch path {
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
	default:
		return ""
	}
}

var statusOutputOrder = []string{
	"connectionToController",
	"cpuUsage",
	"diskUsage",
	"iofogDaemon",
	"memoryUsage",
	"messagesProcessed",
	"runningMicroservices",
	"systemAvailableDisk",
	"systemAvailableMemory",
	"systemTime",
	"systemTotalCpu",
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
	"diagnosticsFrequency",
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
	"diagnosticsFrequency": "postDiagnosticsFrequency",
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
	"postDiagnosticsFreq":    {Key: "postDiagnosticsFreq", Aliases: []string{"df", "-df"}, Type: configValueInt, Help: "diagnostic post frequency (seconds)"},
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

func showMSHelpV3() string {
	return "Usage:\n" +
		"  iofog-agent ms ps\n" +
		"  iofog-agent ms inspect <id>\n" +
		"  iofog-agent ms logs <id>\n" +
		"  iofog-agent ms exec <id> -- <command...>\n" +
		"  iofog-agent ms start|stop|kill|rm <id>"
}

func showDeployHelpV3() string {
	return "Usage:\n" +
		"  iofog-agent deploy -f <manifest.yaml>\n" +
		"  iofog-agent deploy apply -f <manifest.yaml>\n" +
		"  iofog-agent deploy validate -f <manifest.yaml>\n\n" +
		"  iofog-agent deploy registry apply -f <registry.yaml>\n" +
		"  iofog-agent deploy registry validate -f <registry.yaml>\n\n" +
		"Deploys or validates local microservice/registry manifests via LocalAPI v3."
}

func showAuthHelpV3() string {
	return "Usage:\n" +
		"  iofog-agent auth whoami\n" +
		"  iofog-agent auth tokens\n" +
		"  iofog-agent auth revoke <jti>"
}
