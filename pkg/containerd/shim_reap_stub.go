//go:build !linux

package containerd

import (
	"errors"
	"time"
)

const (
	DefaultDataPlaneStopBudget   = 120 * time.Second
	DefaultShimReapBudgetCap     = 30 * time.Second
	DefaultPostStopShimVerifyCap = 10 * time.Second
)

// ReapManagedShimsForSocket is only supported on Linux.
func ReapManagedShimsForSocket(_ string, _ time.Duration) error {
	return errors.New("managed shim reap is only supported on linux")
}

// ReapManagedShimsUntilClear is only supported on Linux.
func ReapManagedShimsUntilClear(_ string, _ time.Duration) error {
	return errors.New("managed shim reap is only supported on linux")
}

// ReapShimsForStaleTask is only supported on Linux.
func ReapShimsForStaleTask(_ string, _ string, _ time.Duration) error {
	return errors.New("managed shim reap is only supported on linux")
}
