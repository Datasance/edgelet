package config

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

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

// ParseSetArgs parses config key/value pairs and validates values client-side.
func ParseSetArgs(args []string) (map[string]interface{}, error) {
	return parseConfigSetArgs(args)
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
