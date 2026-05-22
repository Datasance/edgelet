package run

import "fmt"

// ExecExitError reports a remote command exit status for ms exec.
type ExecExitError struct {
	Code int
}

// NewExecExitError builds an exec exit error.
func NewExecExitError(code int) *ExecExitError {
	return &ExecExitError{Code: code}
}

func (e *ExecExitError) Error() string {
	if e == nil {
		return "command exited"
	}
	return fmt.Sprintf("command exited with status %d", e.Code)
}

func (e *ExecExitError) ExitCode() int {
	if e == nil {
		return ExitInternal
	}
	if e.Code < 0 {
		return ExitInternal
	}
	return e.Code
}
