package output

import (
	"fmt"
	"strings"
)

// MapValueAsString extracts a display string from an API map value.
func MapValueAsString(input map[string]interface{}, key string) string {
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

// MapValueAsRawString extracts a raw string without unknown placeholders.
func MapValueAsRawString(input map[string]interface{}, key string) string {
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

// ValueOrDefault returns value unless empty or unknown.
func ValueOrDefault(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "<unknown>" {
		return fallback
	}
	return value
}
