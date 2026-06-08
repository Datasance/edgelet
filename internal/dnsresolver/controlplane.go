package dnsresolver

// Control plane bridge-network alias tokens (short names; zone suffix added by aliasesForWorkload).
const controlPlaneAliasEdgeletController = "edgelet.controller"

// ControlPlaneBridgeAliasEdgeletController is FQDN #1: edgelet.controller.svc.bridge.local.
func ControlPlaneBridgeAliasEdgeletController() string {
	return controlPlaneAliasEdgeletController
}

// ControlPlaneBridgeAliasNamespaceController is the short alias for FQDN #2:
// controller.<namespace>.svc.bridge.local.
func ControlPlaneBridgeAliasNamespaceController(namespace string) string {
	namespace = normalizeName(namespace)
	if namespace == "" {
		return ""
	}
	return "controller." + namespace
}

// WorkloadBridgeNetworkAliases returns Docker/Podman network DNS alias tokens for a workload.
// For control plane workloads, includes edgelet.controller and controller.<namespace> in addition
// to <namespace>.<name>.
func WorkloadBridgeNetworkAliases(application, name string, isController bool) []string {
	application = normalizeName(application)
	name = normalizeName(name)
	out := make([]string, 0, 4)
	seen := make(map[string]struct{}, 4)
	add := func(alias string) {
		alias = normalizeName(alias)
		if alias == "" {
			return
		}
		if _, ok := seen[alias]; ok {
			return
		}
		seen[alias] = struct{}{}
		out = append(out, alias)
	}

	if application != "" && name != "" {
		add(application + "." + name)
	}
	if isController {
		add(controlPlaneAliasEdgeletController)
		add(ControlPlaneBridgeAliasNamespaceController(application))
	}
	return out
}

// ControlPlaneFQDNs returns the three locked control-plane FQDNs for tests and operator docs.
func ControlPlaneFQDNs(namespace, name string) []string {
	namespace = normalizeName(namespace)
	name = normalizeName(name)
	out := []string{
		controlPlaneAliasEdgeletController + "." + defaultZoneName,
	}
	if ns := ControlPlaneBridgeAliasNamespaceController(namespace); ns != "" {
		out = append(out, ns+"."+defaultZoneName)
	}
	if namespace != "" && name != "" {
		out = append(out, namespace+"."+name+"."+defaultZoneName)
	}
	return out
}
