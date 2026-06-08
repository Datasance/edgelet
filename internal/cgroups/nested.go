package cgroups

import "strings"

// edgeletSystemdUnits are cgroup path segments for edgelet's own systemd units.
// They must not be treated as foreign container runtime indicators.
var edgeletSystemdUnits = []string{
	"edgelet-containerd.service",
	"edgelet-containerd.scope",
	"edgelet.service",
	"edgelet.scope",
}

var foreignCgroupTokens = []string{
	"docker",
	"lxc",
	"libpod",
	"kubepods",
	"podman",
	"cri-containerd",
}

// cgroupPathIndicatesNested reports whether a unified cgroup path suggests the
// process runs inside a foreign container runtime. Edgelet's own systemd slices
// (edgelet-containerd.service, edgelet.service) are never nested.
func cgroupPathIndicatesNested(unifiedPath string) bool {
	path := strings.ToLower(strings.TrimSpace(unifiedPath))
	if isEdgeletSystemdCgroup(path) {
		return false
	}
	path = sanitizeCgroupPathForNestedCheck(path)
	for _, token := range foreignCgroupTokens {
		if strings.Contains(path, token) {
			return true
		}
	}
	return strings.Contains(path, "containerd")
}

func isEdgeletSystemdCgroup(path string) bool {
	for _, unit := range edgeletSystemdUnits {
		if strings.Contains(path, unit) {
			return true
		}
	}
	return false
}

func sanitizeCgroupPathForNestedCheck(path string) string {
	path = strings.ToLower(strings.TrimSpace(path))
	for _, unit := range edgeletSystemdUnits {
		path = strings.ReplaceAll(path, unit, "")
	}
	return path
}
