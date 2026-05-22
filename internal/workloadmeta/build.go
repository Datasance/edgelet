package workloadmeta

import (
	"fmt"
	"sort"
	"strings"
)

type BuildInput struct {
	MicroserviceUUID string
	MicroserviceName string
	ApplicationName  string
	NodeUUID         string
	RuntimeEngine    string

	IsRouter    bool
	IsNats      bool
	HostNetwork bool
	IsSystem    bool

	SandboxID string
	TimeZone  string

	UserEnv    map[string]string
	UserLabels map[string]string
}

func BuildLabels(in BuildInput) map[string]string {
	scope := ResolveScope(in.ApplicationName, in.HostNetwork)
	labels := map[string]string{
		LabelAppName:         strings.TrimSpace(in.MicroserviceName),
		LabelAppInstance:     strings.TrimSpace(in.MicroserviceUUID),
		LabelAppPartOf:       strings.TrimSpace(in.ApplicationName),
		LabelAppManagedBy:    ManagedByValue,
		LabelMicroserviceUID: strings.TrimSpace(in.MicroserviceUUID),
		LabelNodeUID:         strings.TrimSpace(in.NodeUUID),
		LabelScope:           scope,
		LabelRuntimeEngine:   normalizeRuntimeEngine(in.RuntimeEngine),
		LabelRole:            RoleFromMicroservice(in.IsRouter, in.IsNats),
		LabelSystem:          boolLabel(in.IsSystem),
		LabelHostNetwork:     boolLabel(in.HostNetwork),
	}

	if strings.TrimSpace(in.SandboxID) != "" {
		labels[LabelSandboxID] = strings.TrimSpace(in.SandboxID)
	}

	return MergeUserLabels(in.UserLabels, labels)
}

func BuildEnv(in BuildInput) []string {
	scope := ResolveScope(in.ApplicationName, in.HostNetwork)
	canonical := map[string]string{
		EnvMicroserviceUID:  strings.TrimSpace(in.MicroserviceUUID),
		EnvMicroserviceName: strings.TrimSpace(in.MicroserviceName),
		EnvApplicationName:  strings.TrimSpace(in.ApplicationName),
		EnvNodeUID:          strings.TrimSpace(in.NodeUUID),
		EnvScope:            scope,
		EnvRuntimeEngine:    normalizeRuntimeEngine(in.RuntimeEngine),
		EnvRole:             RoleFromMicroservice(in.IsRouter, in.IsNats),
	}

	// Canonical env vars first in deterministic order.
	env := make([]string, 0, len(canonical)+len(in.UserEnv)+1)
	canonicalOrder := []string{
		EnvMicroserviceUID,
		EnvMicroserviceName,
		EnvApplicationName,
		EnvNodeUID,
		EnvScope,
		EnvRuntimeEngine,
		EnvRole,
	}
	for _, key := range canonicalOrder {
		env = append(env, fmt.Sprintf("%s=%s", key, canonical[key]))
	}

	// User env vars are allowed except reserved keys (agent-controlled).
	userKeys := make([]string, 0, len(in.UserEnv))
	for key := range in.UserEnv {
		if _, reserved := ReservedEnvKeys[key]; reserved {
			continue
		}
		userKeys = append(userKeys, key)
	}
	sort.Strings(userKeys)

	hasTZ := false
	for _, key := range userKeys {
		if strings.EqualFold(strings.TrimSpace(key), EnvTimeZone) {
			hasTZ = true
		}
		env = append(env, fmt.Sprintf("%s=%s", key, in.UserEnv[key]))
	}

	if !hasTZ {
		tz := strings.TrimSpace(in.TimeZone)
		if tz == "" {
			tz = "UTC"
		}
		env = append(env, fmt.Sprintf("%s=%s", EnvTimeZone, tz))
	}

	return env
}

func MergeUserLabels(user map[string]string, canonical map[string]string) map[string]string {
	out := make(map[string]string, len(user)+len(canonical))

	for key, value := range user {
		k := strings.TrimSpace(strings.ToLower(key))
		if k == "" {
			continue
		}
		if _, protected := ProtectedLabelKeys[k]; protected {
			continue
		}
		out[k] = strings.TrimSpace(value)
	}

	for key, value := range canonical {
		out[key] = strings.TrimSpace(value)
	}

	return out
}

func normalizeRuntimeEngine(v string) string {
	return strings.ToLower(strings.TrimSpace(v))
}

func boolLabel(v bool) string {
	if v {
		return "true"
	}
	return "false"
}
