//go:build linux && !cgo

package cgroups

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/datasance/edgelet/internal/utils/logging"
)

const (
	defaultUnifiedMount = "/sys/fs/cgroup"
	hybridUnifiedMount  = "/sys/fs/cgroup/unified"
)

var (
	readFileFn            = os.ReadFile
	writeFileFn           = os.WriteFile
	statFn                = os.Stat
	pid1CommFn            = readPID1Comm
	rootHasControllerHost = rootHasControllerFromMount
)

func detectModeHost() (Mode, string, string) {
	if st, err := statFn(filepath.Join(hybridUnifiedMount, "cgroup.controllers")); err == nil && st.IsDir() {
		warn := "hybrid cgroup v1+v2 detected; edgelet prefers unified v2 — consider switching the host to pure cgroup v2"
		logging.LogWarn("Cgroups", warn)
		return ModeHybrid, hybridUnifiedMount, warn
	}
	if _, err := statFn(filepath.Join(defaultUnifiedMount, "cgroup.controllers")); err == nil {
		return ModeV2, defaultUnifiedMount, ""
	}
	for _, name := range []string{"cpu", "memory"} {
		if st, err := statFn(filepath.Join(defaultUnifiedMount, name)); err == nil && st.IsDir() {
			return ModeV1, defaultUnifiedMount, ""
		}
	}
	return ModeUnknown, defaultUnifiedMount, ""
}

func unifiedMountHost(mode Mode) string {
	if mode == ModeHybrid {
		if st, err := statFn(hybridUnifiedMount); err == nil && st.IsDir() {
			return hybridUnifiedMount
		}
	}
	return defaultUnifiedMount
}

func detectNestedHost() bool {
	for _, marker := range []string{"/.dockerenv", "/run/.containerenv"} {
		if _, err := statFn(marker); err == nil {
			return true
		}
	}
	unified, err := parseCgroupFileUnified("/proc/self/cgroup")
	if err != nil {
		return false
	}
	return cgroupPathIndicatesNested(unified)
}

func parseCgroupFileUnified(path string) (string, error) {
	raw, err := readFileFn(path)
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
	return "", fmt.Errorf("unified cgroup path not found in %s", path)
}

func selectDriverHost(mode Mode, nested bool, delegated map[string]bool) (Driver, bool) {
	_ = delegated
	if nested {
		return DriverCgroupfs, false
	}
	if !pid1IsSystemd() {
		return DriverCgroupfs, false
	}
	if mode == ModeV1 {
		return DriverSystemd, false
	}
	mount := unifiedMountHost(mode)
	if mode != ModeUnknown && runningUnderSystemdService() && rootHasControllerHost(mount, mode, "cpuset") {
		return DriverSystemd, false
	}
	return DriverCgroupfs, false
}

func rootHasControllerFromMount(mount string, mode Mode, controller string) bool {
	switch mode {
	case ModeV1:
		st, err := statFn(filepath.Join(defaultUnifiedMount, controller))
		return err == nil && st.IsDir()
	case ModeV2, ModeHybrid:
		raw, err := readFileFn(filepath.Join(mount, "cgroup.controllers"))
		if err != nil {
			return false
		}
		for _, c := range strings.Fields(string(raw)) {
			if c == controller {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func runningUnderSystemdService() bool {
	return os.Getenv("INVOCATION_ID") != ""
}

func pid1IsSystemd() bool {
	if statSystemdRuntime() {
		return true
	}
	comm, err := pid1CommFn()
	if err != nil {
		return false
	}
	return comm == "systemd"
}

func statSystemdRuntime() bool {
	st, err := statFn("/run/systemd/system")
	return err == nil && st.IsDir()
}

func readPID1Comm() (string, error) {
	raw, err := readFileFn("/proc/1/comm")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(raw)), nil
}

func checkDelegatedControllersForMode(mount string, mode Mode) []string {
	if mode == ModeV1 {
		return legacyControllers()
	}

	selfPath, err := currentUnifiedCgroupPath(mount)
	if err != nil {
		return nil
	}
	controllersFile := filepath.Join(mount, strings.TrimPrefix(selfPath, "/"), "cgroup.controllers")
	raw, err := readFileFn(controllersFile)
	if err != nil {
		return nil
	}
	fields := strings.Fields(string(raw))
	out := make([]string, 0, len(fields))
	set := map[string]struct{}{}
	for _, c := range fields {
		set[c] = struct{}{}
		out = append(out, c)
	}
	for _, required := range []string{"cpu", "memory", "pids"} {
		if _, ok := set[required]; ok {
			continue
		}
		if hasSubtreesEnabled(mount, selfPath, required) {
			out = append(out, required)
		}
	}
	return uniqueSorted(out)
}

func legacyControllers() []string {
	controllers := []string{}
	for _, name := range []string{"cpu", "cpuacct", "memory", "pids", "cpuset"} {
		if st, err := statFn(filepath.Join(defaultUnifiedMount, name)); err == nil && st.IsDir() {
			controllers = append(controllers, name)
		}
	}
	return uniqueSorted(controllers)
}

func currentUnifiedCgroupPath(mount string) (string, error) {
	raw, err := readFileFn("/proc/self/cgroup")
	if err != nil {
		return "", err
	}
	scanner := bufio.NewScanner(strings.NewReader(string(raw)))
	for scanner.Scan() {
		parts := strings.SplitN(scanner.Text(), ":", 3)
		if len(parts) < 3 {
			continue
		}
		if parts[0] == "0" && parts[1] == "" {
			path := parts[2]
			if path == "/" {
				return "/", nil
			}
			return strings.TrimPrefix(path, "/"), nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", errors.New("unified cgroup path not found in /proc/self/cgroup")
}

func hasSubtreesEnabled(mount, selfPath, controller string) bool {
	path := filepath.Join(mount, strings.TrimPrefix(selfPath, "/"), "cgroup.subtree_control")
	raw, err := readFileFn(path)
	if err != nil {
		return false
	}
	return strings.Contains(string(raw), controller)
}

func delegatedSet(controllers []string) map[string]bool {
	set := make(map[string]bool, len(controllers))
	for _, c := range controllers {
		set[c] = true
	}
	return set
}

func cgroupPaths(mode Mode, driver Driver) (agentPath, containerdPath string) {
	_, _ = mode, driver
	agentPath = "/edgelet/agent"
	containerdPath = "/edgelet/agent/containerd"
	return agentPath, containerdPath
}

func uniqueSorted(in []string) []string {
	set := map[string]struct{}{}
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		set[v] = struct{}{}
	}
	out := make([]string, 0, len(set))
	for v := range set {
		out = append(out, v)
	}
	sortStrings(out)
	return out
}

func sortStrings(in []string) {
	for i := 0; i < len(in); i++ {
		for j := i + 1; j < len(in); j++ {
			if in[j] < in[i] {
				in[i], in[j] = in[j], in[i]
			}
		}
	}
}
