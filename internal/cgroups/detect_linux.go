//go:build linux

package cgroups

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	cgv3 "github.com/containerd/cgroups/v3"
	"github.com/containerd/cgroups/v3/cgroup1"
	cgv2 "github.com/containerd/cgroups/v3/cgroup2"
	"github.com/datasance/edgelet/internal/utils/logging"
	specs "github.com/opencontainers/runtime-spec/specs-go"
)

const (
	defaultUnifiedMount = "/sys/fs/cgroup"
	hybridUnifiedMount  = "/sys/fs/cgroup/unified"
	containerdGroupName = "containerd"
)

var (
	readFileFn          = os.ReadFile
	writeFileFn         = os.WriteFile
	statFn              = os.Stat
	pid1CommFn          = readPID1Comm
	unifiedMountFn      = unifiedMountpoint
	checkControllersFn  = checkDelegatedControllers
	rootHasControllerFn = rootHasController
)

// Detect inspects the host and returns a recommended cgroup policy.
func Detect() (*CgroupPolicy, error) {
	mode, mount, hybridWarning := detectMode()
	nested := detectNested()
	delegatedList := checkControllersFn(mount)
	delegated := delegatedSet(delegatedList)

	driver, systemdCgroup := selectDriver(mode, nested, delegated)

	agentPath, containerdPath := cgroupPaths(mode, driver)

	return &CgroupPolicy{
		Mode:                 mode,
		Driver:               driver,
		Nested:               nested,
		SystemdCgroup:        systemdCgroup,
		UnifiedMountpoint:    mount,
		AgentCgroupPath:      agentPath,
		ContainerdCgroupPath: containerdPath,
		DelegatedControllers: delegatedList,
		HybridWarning:        hybridWarning,
	}, nil
}

func detectMode() (Mode, string, string) {
	switch cgv3.Mode() {
	case cgv3.Unified:
		return ModeV2, defaultUnifiedMount, ""
	case cgv3.Hybrid:
		warn := "hybrid cgroup v1+v2 detected; edgelet prefers unified v2 — consider switching the host to pure cgroup v2"
		logging.LogWarn("Cgroups", warn)
		if st, err := statFn(hybridUnifiedMount); err == nil && st.IsDir() {
			return ModeHybrid, hybridUnifiedMount, warn
		}
		return ModeHybrid, defaultUnifiedMount, warn
	case cgv3.Legacy:
		return ModeV1, defaultUnifiedMount, ""
	default:
		return ModeUnknown, defaultUnifiedMount, ""
	}
}

func unifiedMountpoint() string {
	if cgv3.Mode() == cgv3.Hybrid {
		if st, err := statFn(hybridUnifiedMount); err == nil && st.IsDir() {
			return hybridUnifiedMount
		}
	}
	return defaultUnifiedMount
}

func selectDriver(mode Mode, nested bool, delegated map[string]bool) (Driver, bool) {
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
	// host gate (config_linux.go): INVOCATION_ID set and root cpuset.
	// The daemon stays in edgelet.service; crun uses cgroupfs under that delegated
	// unit — not systemd scopes.
	if mode != ModeUnknown && runningUnderSystemdService() && rootHasControllerFn("cpuset") {
		return DriverSystemd, false
	}
	return DriverCgroupfs, false
}

func rootHasController(controller string) bool {
	switch cgv3.Mode() {
	case cgv3.Legacy:
		st, err := statFn(filepath.Join(defaultUnifiedMount, controller))
		return err == nil && st.IsDir()
	case cgv3.Unified, cgv3.Hybrid:
		mount := unifiedMountFn()
		m, err := cgv2.NewManager(mount, "/", &cgv2.Resources{})
		if err != nil {
			return false
		}
		controllers, err := m.Controllers()
		if err != nil {
			return false
		}
		for _, c := range controllers {
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

func detectNested() bool {
	for _, marker := range []string{"/.dockerenv", "/run/.containerenv"} {
		if _, err := statFn(marker); err == nil {
			return true
		}
	}
	_, unified, err := cgv3.ParseCgroupFileUnified("/proc/self/cgroup")
	if err != nil {
		return false
	}
	path := strings.ToLower(unified)
	for _, token := range []string{"docker", "lxc", "libpod", "containerd", "kubepods", "podman", "cri-containerd"} {
		if strings.Contains(path, token) {
			return true
		}
	}
	return false
}

func checkDelegatedControllers(mount string) []string {
	if cgv3.Mode() == cgv3.Legacy {
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
		// Root may expose controllers via subtree_control even when not listed.
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
	return "", fmt.Errorf("unified cgroup path not found in /proc/self/cgroup")
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

// ValidatePreflight checks delegation requirements before CRI starts.
func ValidatePreflight(policy *CgroupPolicy) error {
	if policy == nil {
		return fmt.Errorf("cgroup policy is not initialized")
	}
	if cgv3.Mode() == cgv3.Legacy {
		return nil
	}
	if policy.Driver == DriverSystemd {
		return nil
	}
	if !policy.Nested {
		return nil
	}
	delegated := delegatedSet(policy.DelegatedControllers)
	for _, required := range []string{"cpu", "memory", "pids"} {
		if delegated[required] {
			continue
		}
		return &ErrDelegation{Controller: required, Nested: policy.Nested, Mode: policy.Mode}
	}
	return nil
}

// EnsureAgentSubtree creates the agent cgroup and moves the current process into it.
func EnsureAgentSubtree(policy *CgroupPolicy) error {
	if policy == nil {
		return fmt.Errorf("cgroup policy is not initialized")
	}
	if cgv3.Mode() != cgv3.Legacy && policy.Nested {
		if err := prepareNestedContainerRoot(policy); err != nil {
			return err
		}
	}
	if err := ValidatePreflight(policy); err != nil {
		return err
	}

	if cgv3.Mode() == cgv3.Legacy {
		return ensureAgentCgroupV1(policy)
	}
	return ensureAgentCgroupV2(policy)
}

func normalizeUnifiedGroupPath(path string) string {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "." || path == "/" {
		return "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return path
}

func ensureAgentCgroupV2(policy *CgroupPolicy) error {
	mount := policy.UnifiedMountpoint
	if mount == "" {
		mount = unifiedMountFn()
	}
	agentPath := normalizeUnifiedGroupPath(policy.AgentCgroupPath)
	containerdPath := normalizeUnifiedGroupPath(policy.ContainerdCgroupPath)

	edgeletParent := strings.TrimPrefix(filepath.Dir(agentPath), "/")
	if policy.Nested && edgeletParent != "" && edgeletParent != "." {
		if err := ensureUnifiedCgroup(mount, "/"+edgeletParent); err != nil {
			return fmt.Errorf("create edgelet parent cgroup /%s: %w", edgeletParent, err)
		}
		if err := prepareV2ParentForChildren(mount, edgeletParent); err != nil {
			return fmt.Errorf("prepare edgelet parent /%s: %w", edgeletParent, err)
		}
	}

	agentMgr, err := cgv2.NewManager(mount, agentPath, &cgv2.Resources{})
	if err != nil {
		return fmt.Errorf("create agent cgroup %s: %w", agentPath, err)
	}
	// Do not enable subtree_control on /edgelet/agent before AddProc: a cgroup with
	// populated subtree_control is an inner node and cannot hold the daemon process.
	if err := agentMgr.AddProc(uint64(os.Getpid())); err != nil {
		return fmt.Errorf("move edgelet daemon into %s: %w", agentPath, err)
	}
	if _, err := cgv2.NewManager(mount, containerdPath, &cgv2.Resources{}); err != nil {
		return fmt.Errorf("create containerd cgroup %s: %w", containerdPath, err)
	}
	return nil
}

func prepareNestedContainerRoot(policy *CgroupPolicy) error {
	mount := policy.UnifiedMountpoint
	if mount == "" {
		mount = unifiedMountFn()
	}
	selfPath, err := currentUnifiedCgroupPath(mount)
	if err != nil {
		return fmt.Errorf("nested cgroup root path: %w", err)
	}
	logging.LogInfo("Cgroups", fmt.Sprintf("preparing nested container cgroup root %q", selfPath))
	if err := prepareV2ParentForChildren(mount, selfPath); err != nil {
		return fmt.Errorf("prepare nested container cgroup root %q: %w", selfPath, err)
	}
	return nil
}

// prepareV2ParentForChildren enables cgroup v2 controller delegation on parent so
// child cgroups can use cpu/memory/pids.
func prepareV2ParentForChildren(mount, relPath string) error {
	rel := normalizeRelUnifiedPath(relPath)
	if !isUnifiedCgroupNode(mount, rel) {
		return nil
	}
	if subtreeControlSatisfied(mount, rel) {
		return nil
	}
	stagingRel := joinRelUnifiedPath(rel, "init")
	if err := ensureUnifiedCgroup(mount, stagingGroupPath(stagingRel)); err != nil {
		return fmt.Errorf("create staging cgroup %s: %w", stagingGroupPath(stagingRel), err)
	}
	if err := enableSubtreeControl(mount, rel); err != nil {
		if err := evacuateUnifiedProcesses(mount, rel, stagingRel); err != nil {
			return fmt.Errorf("evacuate processes from %q: %w", relPath, err)
		}
		if err := enableSubtreeControl(mount, rel); err != nil {
			return fmt.Errorf("enable subtree_control on %q: %w", relPath, err)
		}
	}
	return nil
}

func normalizeRelUnifiedPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" || path == "/" {
		return ""
	}
	return strings.TrimPrefix(filepath.Clean(path), "/")
}

func joinRelUnifiedPath(rel, suffix string) string {
	suffix = strings.TrimPrefix(suffix, "/")
	if rel == "" {
		return suffix
	}
	return filepath.Join(rel, suffix)
}

func stagingGroupPath(stagingRel string) string {
	if stagingRel == "" {
		return "/init"
	}
	return "/" + stagingRel
}

func unifiedCgroupDir(mount, rel string) string {
	rel = strings.Trim(rel, "/")
	if rel == "" {
		return mount
	}
	return filepath.Join(mount, rel)
}

func isUnifiedCgroupNode(mount, rel string) bool {
	_, err := statFn(filepath.Join(unifiedCgroupDir(mount, rel), "cgroup.controllers"))
	return err == nil
}

func subtreeControlSatisfied(mount, rel string) bool {
	ctrlRaw, err := readFileFn(filepath.Join(unifiedCgroupDir(mount, rel), "cgroup.controllers"))
	if err != nil {
		return false
	}
	ctrls := strings.Fields(string(ctrlRaw))
	if len(ctrls) == 0 {
		return false
	}
	subRaw, err := readFileFn(filepath.Join(unifiedCgroupDir(mount, rel), "cgroup.subtree_control"))
	if err != nil {
		return false
	}
	enabled := string(subRaw)
	for _, c := range ctrls {
		if !strings.Contains(enabled, c) {
			return false
		}
	}
	return true
}

func ensureUnifiedCgroup(mount, groupPath string) error {
	if _, err := cgv2.NewManager(mount, groupPath, &cgv2.Resources{}); err != nil {
		rel := normalizeRelUnifiedPath(groupPath)
		if isUnifiedCgroupNode(mount, rel) {
			return nil
		}
		return err
	}
	return nil
}

func enableSubtreeControl(mount, rel string) error {
	ctrlFile := filepath.Join(unifiedCgroupDir(mount, rel), "cgroup.controllers")
	raw, err := readFileFn(ctrlFile)
	if err != nil {
		return err
	}
	ctrls := strings.Fields(string(raw))
	if len(ctrls) == 0 {
		return fmt.Errorf("no controllers listed in %s", ctrlFile)
	}
	line := buildSubtreeEnableLine(ctrls)
	subCtrl := filepath.Join(unifiedCgroupDir(mount, rel), "cgroup.subtree_control")
	return writeFileFn(subCtrl, []byte(line), 0)
}

func buildSubtreeEnableLine(controllers []string) string {
	var b strings.Builder
	for i, c := range controllers {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString("+")
		b.WriteString(c)
	}
	return b.String()
}

func evacuateUnifiedProcesses(mount, fromRel, toRel string) error {
	procsFile := filepath.Join(unifiedCgroupDir(mount, fromRel), "cgroup.procs")
	destFile := filepath.Join(unifiedCgroupDir(mount, toRel), "cgroup.procs")
	raw, err := readFileFn(procsFile)
	if err != nil {
		return err
	}
	for _, field := range strings.Fields(string(raw)) {
		pid, err := strconv.ParseUint(field, 10, 64)
		if err != nil || pid == 0 {
			continue
		}
		_ = writeFileFn(destFile, []byte(field+"\n"), 0)
	}
	return nil
}

func ensureAgentCgroupV1(policy *CgroupPolicy) error {
	path := cgroup1.StaticPath(strings.TrimPrefix(policy.AgentCgroupPath, "/"))
	cg, err := cgroup1.New(path, &specs.LinuxResources{})
	if err != nil {
		return fmt.Errorf("create v1 agent cgroup %s: %w", policy.AgentCgroupPath, err)
	}
	if err := cg.AddProc(uint64(os.Getpid())); err != nil {
		return fmt.Errorf("move edgelet daemon into v1 agent cgroup: %w", err)
	}
	childPath := cgroup1.StaticPath(strings.TrimPrefix(policy.ContainerdCgroupPath, "/"))
	if _, err := cgroup1.New(childPath, &specs.LinuxResources{}); err != nil {
		return fmt.Errorf("create v1 containerd cgroup %s: %w", policy.ContainerdCgroupPath, err)
	}
	return nil
}

// Bootstrap detects policy, ensures the agent subtree, and stores it globally.
func Bootstrap() (*CgroupPolicy, error) {
	policy, err := Detect()
	if err != nil {
		return nil, err
	}
	if policy.HybridWarning != "" {
		logging.LogWarn("Cgroups", policy.HybridWarning)
	}
	if policy.Nested {
		logging.LogInfo("Cgroups", "nested container environment detected; using cgroupfs driver — docker run --privileged is required")
	}
	logging.LogInfo("Cgroups", fmt.Sprintf(
		"cgroup mode=%s driver=%s nested=%t systemd_cgroup=%t",
		policy.Mode, policy.Driver, policy.Nested, policy.SystemdCgroup,
	))
	// Nested and cgroupfs bare-metal hosts use a dedicated agent subtree. Systemd
	// bare-metal hosts stay in edgelet.service.
	if policy.Driver == DriverCgroupfs {
		if err := EnsureAgentSubtree(policy); err != nil {
			return nil, err
		}
		policy.DelegatedControllers = checkControllersFn(policy.UnifiedMountpoint)
	}
	SetGlobalPolicy(policy)
	return policy, nil
}
