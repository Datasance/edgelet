package run

import (
	"errors"
	"fmt"
)

// ErrHumanOutputWritten marks failures whose human-mode message was already printed to stderr.
var ErrHumanOutputWritten = errors.New("cli: human output already written")

// Error codes aligned with LocalAPI v3 and CLI exit mapping.
const (
	CodeInvalidArgument    = "INVALID_ARGUMENT"
	CodeUnauthorized       = "UNAUTHORIZED"
	CodeForbidden          = "FORBIDDEN"
	CodeNotFound           = "NOT_FOUND"
	CodeConflict           = "CONFLICT"
	CodeNotImplemented     = "NOT_IMPLEMENTED"
	CodeDaemonUnavailable  = "DAEMON_UNAVAILABLE"
	CodeInternal           = "INTERNAL"
)

// ExitCoder allows errors to specify a process exit code.
type ExitCoder interface {
	error
	ExitCode() int
}

// CLIError is a typed CLI failure with code and exit mapping.
type CLIError struct {
	Code    string
	Message string
	Err     error
}

func (e *CLIError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return fmt.Sprintf("Error[%s]: %s", e.Code, e.Message)
	}
	if e.Err != nil {
		return fmt.Sprintf("Error[%s]: %v", e.Code, e.Err)
	}
	return fmt.Sprintf("Error[%s]", e.Code)
}

func (e *CLIError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func (e *CLIError) ExitCode() int {
	if e == nil {
		return ExitInternal
	}
	return ExitCodeForCode(e.Code)
}

// NewCLIError builds a CLIError with normalized code.
func NewCLIError(code, message string, err error) *CLIError {
	code = normalizeCode(code)
	return &CLIError{Code: code, Message: message, Err: err}
}

// NewDisplayedCLIError builds a CLIError whose message was already written to stderr.
func NewDisplayedCLIError(code, message string) *CLIError {
	return NewCLIError(code, message, ErrHumanOutputWritten)
}

func normalizeCode(code string) string {
	if code == "" {
		return CodeInternal
	}
	return code
}

// ExitCodeForError resolves an exit code from any error, honoring ExitCoder.
func ExitCodeForError(err error) int {
	if err == nil {
		return 0
	}
	var exitCoder ExitCoder
	if errors.As(err, &exitCoder) {
		return exitCoder.ExitCode()
	}
	return ExitInternal
}
