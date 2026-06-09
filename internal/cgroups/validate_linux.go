//go:build linux

package cgroups

import (
	"errors"
	"os"
)

// ValidatePreflightLight checks cgroup detectability for init start_pre hooks (no mutation).
func ValidatePreflightLight(policy *CgroupPolicy) error {
	if policy == nil {
		return errors.New("cgroup policy is not initialized")
	}
	if policy.Mode == ModeUnknown {
		return errors.New("cgroup mode could not be detected")
	}
	mount := policy.UnifiedMountpoint
	if mount == "" {
		mount = "/sys/fs/cgroup"
	}
	if st, err := os.Stat(mount); err != nil || !st.IsDir() {
		return errors.New("cgroup filesystem is not mounted at " + mount)
	}
	return nil
}

// ValidatePreflight checks delegation requirements after bootstrap prep (fat runtime).
func ValidatePreflight(policy *CgroupPolicy) error {
	if policy == nil {
		return errors.New("cgroup policy is not initialized")
	}
	if policy.Mode == ModeV1 {
		return nil
	}
	if policy.Driver == DriverSystemd {
		return nil
	}
	if !policy.Nested && !policy.MachineRoot {
		return nil
	}
	delegated := delegatedSet(policy.DelegatedControllers)
	for _, required := range []string{"cpu", "memory", "pids"} {
		if delegated[required] {
			continue
		}
		return &ErrDelegation{
			Controller:  required,
			Nested:      policy.Nested,
			MachineRoot: policy.MachineRoot,
			Mode:        policy.Mode,
		}
	}
	return nil
}
