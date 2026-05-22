package workloadmeta

import "strings"

const (
	ManagedByValue = "iofog-agent"
)

const (
	RuntimeEngineDocker = "docker"
	RuntimeEnginePodman = "podman"
	RuntimeEngineIofog  = "iofog"
)

const (
	RoleWorkload = "workload"
	RoleRouter   = "router"
	RoleNats     = "nats"
)

const (
	ScopeManaged = "managed"
	ScopeLocal   = "local"
)

const (
	LocalApplicationName = "local"
)

const (
	LabelAppName      = "app.kubernetes.io/name"
	LabelAppInstance  = "app.kubernetes.io/instance"
	LabelAppPartOf    = "app.kubernetes.io/part-of"
	LabelAppManagedBy = "app.kubernetes.io/managed-by"

	LabelMicroserviceUID = "iofog.org/microservice-uid"
	LabelNodeUID         = "iofog.org/node-uid"
	LabelScope           = "iofog.org/scope"
	LabelRuntimeEngine   = "iofog.org/runtime-engine"
	LabelRole            = "iofog.org/role"

	LabelSystem      = "iofog.org/system"
	LabelHostNetwork = "iofog.org/host-network"
	LabelSandboxID   = "iofog.org/sandbox-id"
	LabelHealthcheck = "iofog.org/healthcheck"
)

const (
	EnvMicroserviceUID  = "IOFOG_MICROSERVICE_UID"
	EnvMicroserviceName = "IOFOG_MICROSERVICE_NAME"
	EnvApplicationName  = "IOFOG_APPLICATION_NAME"
	EnvNodeUID          = "IOFOG_NODE_UID"
	EnvScope            = "IOFOG_SCOPE"
	EnvRuntimeEngine    = "IOFOG_RUNTIME_ENGINE"
	EnvRole             = "IOFOG_ROLE"
	EnvTimeZone         = "TZ"
)

// ProtectedLabelKeys cannot be overridden by user-supplied labels.
var ProtectedLabelKeys = map[string]struct{}{
	LabelAppName:         {},
	LabelAppInstance:     {},
	LabelAppPartOf:       {},
	LabelAppManagedBy:    {},
	LabelMicroserviceUID: {},
	LabelNodeUID:         {},
	LabelScope:           {},
	LabelRuntimeEngine:   {},
	LabelRole:            {},
	LabelSystem:          {},
	LabelHostNetwork:     {},
	LabelSandboxID:       {},
	LabelHealthcheck:     {},
}

// ReservedEnvKeys are agent-controlled and cannot be overridden by user env.
var ReservedEnvKeys = map[string]struct{}{
	EnvMicroserviceUID:  {},
	EnvMicroserviceName: {},
	EnvApplicationName:  {},
	EnvNodeUID:          {},
	EnvScope:            {},
	EnvRuntimeEngine:    {},
	EnvRole:             {},
}

// RemovedLegacyLabelKeys lists identity/metadata label keys retired in favor of LabelSpec v1.
// Retained for documentation, tests, and repository audits (not used at runtime).
var RemovedLegacyLabelKeys = []string{
	"iofog-ms",
	"iofog-name",
	"iofog-app",
	"iofog-uuid",
	"iofog.uuid",
	"iofog-router",
	"iofog-nats",
	"iofog-system",
	"iofog-hostnet",
	"iofog-sandbox-id",
	"iofog-healthcheck",
}

// RemovedLegacyEnvVars lists predefined env keys retired in favor of EnvSpec v1.
var RemovedLegacyEnvVars = []string{
	"SELFNAME",
}

func MicroserviceUIDFromLabels(labels map[string]string) string {
	if labels == nil {
		return ""
	}
	return strings.TrimSpace(labels[LabelMicroserviceUID])
}

func IsManagedByIofog(labels map[string]string) bool {
	if labels == nil {
		return false
	}
	managedBy := strings.TrimSpace(labels[LabelAppManagedBy])
	return strings.EqualFold(managedBy, ManagedByValue) && MicroserviceUIDFromLabels(labels) != ""
}

func IsSystemWorkload(labels map[string]string) bool {
	if labels == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(labels[LabelSystem]), "true")
}

func RoleFromMicroservice(isRouter, isNats bool) string {
	// Deterministic precedence: router > nats > workload.
	if isRouter {
		return RoleRouter
	}
	if isNats {
		return RoleNats
	}
	return RoleWorkload
}

func IsLocalApplication(application string) bool {
	return strings.EqualFold(strings.TrimSpace(application), LocalApplicationName)
}

func ResolveScope(application string, hostNetwork bool) string {
	// Host-network workloads always bypass scoped bridge networking.
	if hostNetwork {
		return ScopeManaged
	}
	if IsLocalApplication(application) {
		return ScopeLocal
	}
	return ScopeManaged
}

func IsLocalScope(scope string) bool {
	return strings.EqualFold(strings.TrimSpace(scope), ScopeLocal)
}

func ScopeFromMicroservice(application string, hostNetwork bool) string {
	return ResolveScope(application, hostNetwork)
}
