//go:build !linux

package cgroups

import "fmt"

// Detect is unsupported off Linux.
func Detect() (*CgroupPolicy, error) {
	return nil, fmt.Errorf("cgroups bootstrap is only supported on linux")
}

// Bootstrap is unsupported off Linux.
func Bootstrap() (*CgroupPolicy, error) {
	return nil, fmt.Errorf("cgroups bootstrap is only supported on linux")
}

// EnsureAgentSubtree is unsupported off Linux.
func EnsureAgentSubtree(_ *CgroupPolicy) error {
	return fmt.Errorf("cgroups bootstrap is only supported on linux")
}

// ValidatePreflight is unsupported off Linux.
func ValidatePreflight(_ *CgroupPolicy) error {
	return nil
}
