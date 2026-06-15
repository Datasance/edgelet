package version

import (
	"errors"
	"fmt"
	"strings"
)

// NormalizeVersionResponse maps flat v3.8 or legacy nested controller version payloads
// to the internal action map consumed by ChangeVersion.
func NormalizeVersionResponse(raw map[string]any) (map[string]any, error) {
	if raw == nil {
		return nil, nil
	}

	if nested, ok := raw["versionCommand"].(map[string]any); ok {
		return normalizeNestedVersionResponse(nested)
	}

	if cmdRaw, ok := raw["versionCommand"].(string); ok {
		return normalizeFlatVersionResponse(cmdRaw, raw)
	}

	return nil, nil
}

func normalizeFlatVersionResponse(cmdRaw string, raw map[string]any) (map[string]any, error) {
	cmd, err := parseVersionCommandString(cmdRaw)
	if err != nil {
		return nil, err
	}

	action := map[string]any{
		"command": string(cmd),
	}
	copyVersionActionFields(action, raw)
	return action, nil
}

func normalizeNestedVersionResponse(nested map[string]any) (map[string]any, error) {
	cmdRaw, ok := nested["command"].(string)
	if !ok || strings.TrimSpace(cmdRaw) == "" {
		return nil, errors.New("command not found in nested versionCommand")
	}

	cmd, err := parseVersionCommandString(cmdRaw)
	if err != nil {
		return nil, err
	}

	action := map[string]any{
		"command": string(cmd),
	}
	copyVersionActionFields(action, nested)
	return action, nil
}

func copyVersionActionFields(action, source map[string]any) {
	if key, ok := source["provisionKey"].(string); ok && strings.TrimSpace(key) != "" {
		action["provisionKey"] = key
	}
	if raw, ok := source["expirationTime"]; ok && raw != nil {
		action["expirationTime"] = raw
	}
	for _, field := range []string{"semver", "version", "targetVersion", "target"} {
		if value, ok := source[field].(string); ok && strings.TrimSpace(value) != "" {
			action[field] = value
		}
	}
}

func parseVersionCommandString(raw string) (VersionCommand, error) {
	cmd := VersionCommand(strings.ToUpper(strings.TrimSpace(raw)))
	switch cmd {
	case VersionCommandUpgrade, VersionCommandRollback:
		return cmd, nil
	default:
		return "", fmt.Errorf("unknown version command: %s", raw)
	}
}
