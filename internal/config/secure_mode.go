package config

import "strings"

// ParseSecureMode interprets secureMode config values. Only explicit off/false
// synonyms disable TLS verification; everything else enables secure mode.
func ParseSecureMode(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "off", "false", "0", "no":
		return false
	default:
		return true
	}
}

// normalizeSecureModeYAML persists canonical on/off strings in config YAML.
func normalizeSecureModeYAML(value string) string {
	if ParseSecureMode(value) {
		return "on"
	}
	return "off"
}
