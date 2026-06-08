//go:build linux

package cgroups

import (
	"errors"
)

// ValidatePreflight checks delegation requirements before CRI starts.
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
