package engine

import (
	"errors"
	"fmt"
	"strings"
)

const (
	// CRIReasonContainerExited is reported by CRI when a dead container object
	// cannot be started and must be recreated.
	CRIReasonContainerExited = "CONTAINER_EXITED"
)

// NonRestartableContainerError signals that a runtime container object is in a
// terminal non-restartable state and must be removed/recreated.
type NonRestartableContainerError struct {
	ContainerID string
	Reason      string
	ExitCode    int32
	Message     string
}

func (e *NonRestartableContainerError) Error() string {
	if e == nil {
		return "non-restartable container"
	}
	return fmt.Sprintf(
		"container %s is non-restartable (reason=%s exitCode=%d message=%q)",
		strings.TrimSpace(e.ContainerID),
		strings.TrimSpace(e.Reason),
		e.ExitCode,
		strings.TrimSpace(e.Message),
	)
}

// IsNonRestartableContainerError returns the typed error if err wraps one.
func IsNonRestartableContainerError(err error) (*NonRestartableContainerError, bool) {
	if err == nil {
		return nil, false
	}
	var target *NonRestartableContainerError
	if errors.As(err, &target) {
		return target, true
	}
	return nil, false
}

// IsNonRestartableCRIReason reports whether the CRI reason indicates terminal
// non-restartable state.
func IsNonRestartableCRIReason(reason string) bool {
	return strings.EqualFold(strings.TrimSpace(reason), CRIReasonContainerExited)
}
