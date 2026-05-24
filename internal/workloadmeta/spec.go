package workloadmeta

import "strings"

const (
	ManagedByValue = "edgelet"
)

const (
	RuntimeEngineDocker = "docker"
	RuntimeEnginePodman = "podman"
	RuntimeEngineEdgelet  = "edgelet"
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

	LabelMicroserviceUID = "edgelet.iofog.org/microservice-uid"
	LabelNodeUID         = "edgelet.iofog.org/node-uid"
	LabelScope           = "edgelet.iofog.org/scope"
	LabelRuntimeEngine   = "edgelet.iofog.org/runtime-engine"
	LabelRole            = "edgelet.iofog.org/role"

	LabelSystem      = "edgelet.iofog.org/system"
	LabelHostNetwork = "edgelet.iofog.org/host-network"
	LabelSandboxID   = "edgelet.iofog.org/sandbox-id"
	LabelHealthcheck = "edgelet.iofog.org/healthcheck"
)

const (
	EnvMicroserviceUID  = "EDGELET_MICROSERVICE_UID"
	EnvMicroserviceName = "EDGELET_MICROSERVICE_NAME"
	EnvApplicationName  = "EDGELET_APPLICATION_NAME"
	EnvNodeUID          = "EDGELET_NODE_UID"
	EnvScope            = "EDGELET_SCOPE"
	EnvRuntimeEngine    = "EDGELET_RUNTIME_ENGINE"
	EnvRole             = "EDGELET_ROLE"
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
