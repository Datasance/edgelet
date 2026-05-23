package run

import "strings"

const (
	ExitSuccess           = 0
	ExitInternal          = 1
	ExitInvalidArgument   = 2
	ExitUnauthorized      = 3
	ExitNotFound          = 4
	ExitConflict          = 5
	ExitNotImplemented    = 6
	ExitDaemonUnavailable = 10
)

// ExitCodeForCode maps API/CLI error codes to process exit codes.
func ExitCodeForCode(code string) int {
	switch strings.ToUpper(strings.TrimSpace(code)) {
	case CodeInvalidArgument:
		return ExitInvalidArgument
	case CodeUnauthorized, CodeForbidden:
		return ExitUnauthorized
	case CodeNotFound:
		return ExitNotFound
	case CodeConflict:
		return ExitConflict
	case CodeNotImplemented:
		return ExitNotImplemented
	case CodeDaemonUnavailable:
		return ExitDaemonUnavailable
	default:
		return ExitInternal
	}
}
