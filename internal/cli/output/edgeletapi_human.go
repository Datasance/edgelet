package output

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

var statusOutputOrder = []string{
	"connectionToController",
	"cpuUsage",
	"diskUsage",
	"edgeletDaemon",
	"memoryUsage",
	"runningMicroservices",
	"systemAvailableDisk",
	"systemAvailableMemory",
	"systemTime",
	"systemTotalCpu",
	"availableNetworkInterfaces",
	"availableRuntimes",
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
	"containerEngineUrl",
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
}

// FormatEdgeletAPIHuman renders human-readable output for a EdgeletAPI v1 route payload.
func FormatEdgeletAPIHuman(routePath string, result map[string]any) string {
	routePath = stripQuery(routePath)
	switch routePath {
	case "/v1/system/status":
		return formatFlatMapWithOrder(result, statusOutputOrder)
	case "/v1/system/info":
		return formatInfoWithAliasOrder(result)
	case "/v1/ms":
		return formatMSList(result)
	case "/v1/images":
		return formatImageList(result)
	case "/v1/deploy/registries":
		return formatRegistryList(result)
	case "/v1/deploy/runtimeclasses":
		return formatRuntimeClassList(result)
	case "/v1/system/controlplane":
		return formatControlPlaneStatus(result)
	case "/v1/system/controlplane/manifest":
		return formatControlPlaneManifest(result)
	default:
		if human := formatMutationRoute(routePath, result); human != "" {
			return human
		}
		if strings.HasPrefix(routePath, "/v1/ms/") {
			return formatMSInspect(result)
		}
		return ""
	}
}

func formatMSInspect(result map[string]any) string {
	if len(result) == 0 {
		return ""
	}
	return formatFlatMapWithOrder(result, nil)
}

func formatFlatMapWithOrder(result map[string]any, preferred []string) string {
	if len(result) == 0 {
		return ""
	}
	for _, value := range result {
		switch value.(type) {
		case map[string]any, []any:
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
		_, _ = fmt.Fprintf(&b, "%s: %v\n", key, value)
		seen[key] = true
	}
	remaining := make([]string, 0, len(result))
	for key := range result {
		if !seen[key] {
			remaining = append(remaining, key)
		}
	}
	slices.Sort(remaining)
	for _, key := range remaining {
		_, _ = fmt.Fprintf(&b, "%s: %v\n", key, result[key])
	}
	return strings.TrimRight(b.String(), "\n")
}

func formatInfoWithAliasOrder(result map[string]any) string {
	if len(result) == 0 {
		return ""
	}
	for _, value := range result {
		switch value.(type) {
		case map[string]any, []any:
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
		_, _ = fmt.Fprintf(&b, "%s: %v\n", alias, value)
		seenCanonical[canonical] = true
	}

	remainingAliases := make([]string, 0, len(result))
	for canonical := range result {
		if seenCanonical[canonical] {
			continue
		}
		remainingAliases = append(remainingAliases, canonical)
	}
	slices.Sort(remainingAliases)
	for _, key := range remainingAliases {
		_, _ = fmt.Fprintf(&b, "%s: %v\n", key, result[key])
	}
	return strings.TrimRight(b.String(), "\n")
}

func formatMSList(result map[string]any) string {
	rawItems, ok := result["items"].([]any)
	if !ok || len(rawItems) == 0 {
		return "No microservices found."
	}
	rows := [][]string{
		{"UUID", "APPLICATIONNAME", "MICROSERVICENAME", "STATE", "CONTAINERID", "IMAGE", "TYPE"},
	}
	for _, raw := range rawItems {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		rows = append(rows, []string{
			MapValueAsString(item, "uuid"),
			MapValueAsString(item, "application"),
			MapValueAsString(item, "name"),
			MapValueAsString(item, "state"),
			MapValueAsString(item, "containerId"),
			MapValueAsString(item, "image"),
			MapValueAsString(item, "type"),
		})
	}
	return formatAlignedTable(rows)
}

func formatRegistryList(result map[string]any) string {
	rawItems, ok := result["items"].([]any)
	if !ok || len(rawItems) == 0 {
		return "No registries found."
	}
	rows := [][]string{
		{"ID", "URL", "PUBLIC", "USERNAME", "EMAIL"},
	}
	for _, raw := range rawItems {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		rows = append(rows, []string{
			MapValueAsString(item, "id"),
			MapValueAsString(item, "url"),
			MapValueAsString(item, "isPublic"),
			MapValueAsString(item, "userName"),
			MapValueAsString(item, "userEmail"),
		})
	}
	return formatAlignedTable(rows)
}

func formatRuntimeClassList(result map[string]any) string {
	rawItems, ok := result["items"].([]any)
	if !ok || len(rawItems) == 0 {
		return "No runtime classes found."
	}
	rows := [][]string{
		{"NAME", "HANDLER", "RUNTIME"},
	}
	for _, raw := range rawItems {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		rows = append(rows, []string{
			MapValueAsString(item, "name"),
			MapValueAsString(item, "handler"),
			MapValueAsString(item, "runtimeName"),
		})
	}
	return formatAlignedTable(rows)
}

var controlPlaneStatusOrder = []string{
	"controllerUuid",
	"namespace",
	"name",
	"image",
	"containerId",
	"state",
	"desiredState",
	"runtimeState",
	"generation",
	"observedGeneration",
	"restartCount",
	"lastError",
	"lastTransitionAt",
	"source",
	"type",
}

func formatControlPlaneStatus(result map[string]any) string {
	if _, hasUUID := result["controllerUuid"]; !hasUUID {
		if MapValueAsString(result, "status") == "ok" {
			return "control plane deployment removed successfully"
		}
		return formatFlatMapWithOrder(result, nil)
	}
	return formatFlatMapWithOrder(result, controlPlaneStatusOrder)
}

func formatControlPlaneManifest(result map[string]any) string {
	yaml := strings.TrimSpace(MapValueAsString(result, "manifestYaml"))
	if yaml == "" {
		return "control plane manifest unavailable"
	}
	masked := MapValueAsString(result, "masked")
	header := "manifestYaml:"
	if masked == "true" {
		header = "manifestYaml (secrets masked):"
	}
	return header + "\n" + yaml
}

func formatImageList(result map[string]any) string {
	rawItems, ok := result["items"].([]any)
	if !ok || len(rawItems) == 0 {
		return "No images found."
	}
	rows := [][]string{
		{"REPOSITORY", "TAG", "IMAGE ID", "CREATED", "DISK USAGE", "CONTENT SIZE", "IN USE"},
	}
	for _, raw := range rawItems {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		rows = append(rows, []string{
			ValueOrDefault(MapValueAsString(item, "repository"), "<none>"),
			ValueOrDefault(MapValueAsString(item, "tag"), "<none>"),
			ValueOrDefault(MapValueAsString(item, "shortId"), "<none>"),
			humanizeCreated(MapValueAsString(item, "createdAt")),
			ValueOrDefault(MapValueAsString(item, "diskUsageHuman"), "0 B"),
			formatOptionalHumanSize(MapValueAsString(item, "contentSizeHuman")),
			formatOptionalInUse(item["inUse"]),
		})
	}
	return formatAlignedTable(rows)
}

func formatOptionalHumanSize(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "<unknown>" {
		return "-"
	}
	return value
}

func formatOptionalInUse(raw any) string {
	switch v := raw.(type) {
	case nil:
		return "-"
	case float64:
		return strconv.FormatInt(int64(v), 10)
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case json.Number:
		return v.String()
	default:
		s := strings.TrimSpace(fmt.Sprint(v))
		if s == "" || s == "<nil>" {
			return "-"
		}
		return s
	}
}

func stripQuery(path string) string {
	if idx := strings.Index(path, "?"); idx >= 0 {
		return path[:idx]
	}
	return path
}
