package output

import (
	"fmt"
	"sort"
	"strings"
)

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

// FormatEdgeletAPIHuman renders human-readable output for a EdgeletAPI v1 route payload.
func FormatEdgeletAPIHuman(routePath string, result map[string]interface{}) string {
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

func formatMSInspect(result map[string]interface{}) string {
	if len(result) == 0 {
		return ""
	}
	return formatFlatMapWithOrder(result, nil)
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
			MapValueAsString(item, "id"),
			MapValueAsString(item, "url"),
			MapValueAsString(item, "isPublic"),
			MapValueAsString(item, "userName"),
			MapValueAsString(item, "userEmail"),
		})
	}
	return formatAlignedTable(rows)
}

func formatRuntimeClassList(result map[string]interface{}) string {
	rawItems, ok := result["items"].([]interface{})
	if !ok || len(rawItems) == 0 {
		return "No runtime classes found."
	}
	rows := [][]string{
		{"NAME", "HANDLER", "RUNTIME"},
	}
	for _, raw := range rawItems {
		item, ok := raw.(map[string]interface{})
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

func formatControlPlaneStatus(result map[string]interface{}) string {
	if _, hasUUID := result["controllerUuid"]; !hasUUID {
		if MapValueAsString(result, "status") == "ok" {
			return "control plane deployment removed successfully"
		}
		return formatFlatMapWithOrder(result, nil)
	}
	return formatFlatMapWithOrder(result, controlPlaneStatusOrder)
}

func formatControlPlaneManifest(result map[string]interface{}) string {
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
			ValueOrDefault(MapValueAsString(item, "repository"), "<none>"),
			ValueOrDefault(MapValueAsString(item, "tag"), "<none>"),
			ValueOrDefault(MapValueAsString(item, "shortId"), "<none>"),
			humanizeCreated(MapValueAsString(item, "createdAt")),
			ValueOrDefault(MapValueAsString(item, "sizeHuman"), "0 B"),
		})
	}
	return formatAlignedTable(rows)
}

func stripQuery(path string) string {
	if idx := strings.Index(path, "?"); idx >= 0 {
		return path[:idx]
	}
	return path
}
