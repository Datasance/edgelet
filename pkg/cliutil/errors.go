package cliutil

import "fmt"

// InputError indicates invalid user input.
type InputError struct {
	Message string
}

func NewInputError(message string) *InputError {
	return &InputError{Message: message}
}

func (e *InputError) Error() string {
	if e == nil || e.Message == "" {
		return "invalid input"
	}
	return e.Message
}

// NotFoundError indicates a missing resource.
type NotFoundError struct {
	Message string
}

func NewNotFoundError(message string) *NotFoundError {
	return &NotFoundError{Message: message}
}

func (e *NotFoundError) Error() string {
	if e == nil || e.Message == "" {
		return "not found"
	}
	return e.Message
}

// ConflictError indicates a conflicting state.
type ConflictError struct {
	Message string
}

func NewConflictError(message string) *ConflictError {
	return &ConflictError{Message: message}
}

func (e *ConflictError) Error() string {
	if e == nil || e.Message == "" {
		return "conflict"
	}
	return e.Message
}

// InternalError indicates an unexpected failure.
type InternalError struct {
	Message string
	Err     error
}

func NewInternalError(message string, err error) *InternalError {
	return &InternalError{Message: message, Err: err}
}

func (e *InternalError) Error() string {
	if e == nil {
		return "internal error"
	}
	if e.Message != "" && e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	if e.Message != "" {
		return e.Message
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return "internal error"
}

func (e *InternalError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}
