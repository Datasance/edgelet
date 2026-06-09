package cgroups

import "fmt"

// ErrDelegation indicates required cgroup controllers are not delegated to edgelet.
type ErrDelegation struct {
	Controller  string
	Nested      bool // workload container (docker/k8s dev image)
	MachineRoot bool // LXC/VM machine root without delegation
	Mode        Mode
}

func (e *ErrDelegation) Error() string {
	if e == nil {
		return "cgroup delegation failed"
	}
	base := fmt.Sprintf("controller %s is not available", e.Controller)
	if e.Nested {
		return base + ": nested edgelet container requires `docker run --privileged` so cpu/memory/pids controllers are delegated; see docs/edgelet/cgroups.md"
	}
	if e.MachineRoot {
		return base + ": LXC/VM machine root lacks delegated cgroup controllers; on OpenRC ensure edgelet-cgroup-prep runs at sysinit (reinstall edgelet); see docs/edgelet/troubleshooting.md"
	}
	switch e.Mode {
	case ModeHybrid:
		return base + ": hybrid cgroup layout detected — prefer unified cgroup v2 or ensure controllers are delegated to the edgelet cgroup; see docs/edgelet/troubleshooting.md"
	default:
		return base + ": ensure cgroup controllers are enabled and delegated; see docs/edgelet/troubleshooting.md"
	}
}

// MapRuntimeError converts common CRI/crun delegation failures into actionable errors.
func MapRuntimeError(err error, policy *CgroupPolicy) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	nested := policy != nil && policy.Nested
	machineRoot := policy != nil && policy.MachineRoot
	mode := ModeUnknown
	if policy != nil {
		mode = policy.Mode
	}
	for _, ctrl := range []string{"cpu", "memory", "pids", "cpuset", "io"} {
		if containsControllerUnavailable(msg, ctrl) {
			return &ErrDelegation{Controller: ctrl, Nested: nested, MachineRoot: machineRoot, Mode: mode}
		}
	}
	return err
}

func containsControllerUnavailable(msg, controller string) bool {
	needle := fmt.Sprintf("controller %s is not available", controller)
	if stringsContainsFold(msg, needle) {
		return true
	}
	alt := fmt.Sprintf("controller %q is not available", controller)
	return stringsContainsFold(msg, alt)
}

func stringsContainsFold(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (haystack == needle ||
		len(needle) == 0 ||
		containsFoldASCII(haystack, needle))
}

func containsFoldASCII(s, sub string) bool {
	return len(sub) <= len(s) && indexFoldASCII(s, sub) >= 0
}

func indexFoldASCII(s, sub string) int {
outer:
	for i := 0; i+len(sub) <= len(s); i++ {
		for j := 0; j < len(sub); j++ {
			a, b := s[i+j], sub[j]
			if a >= 'A' && a <= 'Z' {
				a += 'a' - 'A'
			}
			if b >= 'A' && b <= 'Z' {
				b += 'a' - 'A'
			}
			if a != b {
				continue outer
			}
		}
		return i
	}
	return -1
}
