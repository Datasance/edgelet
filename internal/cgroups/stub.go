//go:build !linux

package cgroups

import (
	"errors"
)

// Detect is unsupported off Linux.
func Detect() (*CgroupPolicy, error) {
	return nil, errors.New("cgroups bootstrap is only supported on linux")
}

// Bootstrap is unsupported off Linux.
func Bootstrap() (*CgroupPolicy, error) {
	return nil, errors.New("cgroups bootstrap is only supported on linux")
}

// PublishHostPolicy is unsupported off Linux.
func PublishHostPolicy() (*CgroupPolicy, error) {
	return nil, errors.New("cgroups publish is only supported on linux")
}

// EnsureAgentSubtree is unsupported off Linux.
func EnsureAgentSubtree(_ *CgroupPolicy) error {
	return errors.New("cgroups bootstrap is only supported on linux")
}

// ValidatePreflight is unsupported off Linux.
func ValidatePreflight(_ *CgroupPolicy) error {
	return nil
}
