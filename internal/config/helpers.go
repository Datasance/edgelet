package config

import (
	"strings"
	"unicode"
)

// snakeToCamel converts snake_case to camelCase
// Example: "disk_consumption_limit" -> "diskConsumptionLimit"
func snakeToCamel(s string) string {
	if s == "" {
		return s
	}

	parts := strings.Split(s, "_")
	result := strings.Builder{}
	result.WriteString(strings.ToLower(parts[0]))

	for i := 1; i < len(parts); i++ {
		if len(parts[i]) > 0 {
			// Capitalize first letter
			runes := []rune(parts[i])
			if len(runes) > 0 {
				runes[0] = unicode.ToUpper(runes[0])
				result.WriteString(string(runes))
			}
		}
	}

	return result.String()
}
