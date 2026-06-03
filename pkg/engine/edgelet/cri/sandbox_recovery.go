//go:build linux

package cri

import (
	"errors"
	"regexp"
	"strings"

	"google.golang.org/grpc/status"
)

var reservedSandboxIDPattern = regexp.MustCompile(`is reserved for "([^"]+)"`)

// IsPodSandboxNameReserved reports whether err is a CRI name reservation conflict.
func IsPodSandboxNameReserved(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "failedprecondition") && strings.Contains(msg, "reserved for")
}

// ReservedPodSandboxID extracts the blocking sandbox id from a reservation error.
func ReservedPodSandboxID(err error) string {
	if err == nil {
		return ""
	}
	if match := reservedSandboxIDPattern.FindStringSubmatch(err.Error()); len(match) > 1 {
		return strings.TrimSpace(match[1])
	}
	return ""
}

// GRPCStatusCode returns the gRPC status code when present.
func GRPCStatusCode(err error) int {
	if err == nil {
		return 0
	}
	if st, ok := status.FromError(err); ok && st != nil {
		return int(st.Code())
	}
	return 0
}

// IsFailedPrecondition reports whether err is a gRPC FailedPrecondition.
func IsFailedPrecondition(err error) bool {
	if err == nil {
		return false
	}
	if st, ok := status.FromError(err); ok && st != nil {
		return st.Code() == 9 // codes.FailedPrecondition
	}
	return strings.Contains(strings.ToLower(err.Error()), "failedprecondition")
}

// WrapSandboxReleaseError annotates release failures for observability.
func WrapSandboxReleaseError(err error, sandboxID string) error {
	if err == nil {
		return nil
	}
	return errors.Join(err, errors.New("release pod sandbox "+sandboxID))
}
