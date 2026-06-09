package cgroups

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

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
	"libpod",
	"kubepods",
	"podman",
	"cri-containerd",
}

// IsMachineRootCgroupPath reports VM/LXC machine boundary cgroups (not workload containers).
func IsMachineRootCgroupPath(unifiedPath string) bool {
	path := strings.ToLower(strings.TrimSpace(unifiedPath))
	if path == "" || path == "/" {
		return true
	}
	clean := filepath.Clean(path)
	if !strings.HasPrefix(clean, "/") {
		clean = "/" + clean
	}
	switch clean {
	case "/", "/.lxc", "/init.scope":
		return true
	}
	// OrbStack/Moby staging after edgelet-cgroup-prep (e.g. /.lxc/init); not workload-nested.
	if strings.HasPrefix(clean, "/.lxc/") {
		return true
	}
	return false
}

// DetectMachineRoot reports LXC/VM machine-root hosts (e.g. OrbStack /.lxc layout).
func DetectMachineRoot() bool {
	if st, err := os.Stat(filepath.Join("/sys/fs/cgroup", ".lxc")); err == nil && st.IsDir() {
		return true
	}
	if unified, err := readSelfUnifiedCgroupPath(); err == nil {
		return IsMachineRootCgroupPath(unified)
	}
	return false
}

func readSelfUnifiedCgroupPath() (string, error) {
	raw, err := os.ReadFile("/proc/self/cgroup")
	if err != nil {
		return "", err
	}
	scanner := bufio.NewScanner(strings.NewReader(string(raw)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "0::") {
			return strings.TrimPrefix(line, "0::"), nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", os.ErrNotExist
}

// cgroupPathIndicatesNested reports whether a unified cgroup path suggests the
// process runs inside a foreign workload container. Edgelet's own systemd slices
// and machine-root boundaries (/.lxc, /, init.scope) are never workload-nested.
func cgroupPathIndicatesNested(unifiedPath string) bool {
	path := strings.ToLower(strings.TrimSpace(unifiedPath))
	if isEdgeletManagedCgroup(path) {
		return false
	}
	if IsMachineRootCgroupPath(unifiedPath) {
		return false
	}
	path = sanitizeCgroupPathForNestedCheck(path)
	for _, token := range foreignCgroupTokens {
		if strings.Contains(path, token) {
			return true
		}
	}
	if strings.Contains(path, "lxc") {
		return true
	}
	return strings.Contains(path, "containerd")
}

func isEdgeletManagedCgroup(path string) bool {
	if isEdgeletSystemdCgroup(path) {
		return true
	}
	return strings.Contains(path, "openrc.edgelet")
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
	path = strings.ReplaceAll(path, "openrc.edgelet-containerd", "")
	path = strings.ReplaceAll(path, "openrc.edgelet", "")
	return path
}
