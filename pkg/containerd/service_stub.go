//go:build !linux

// Package edgeletcontainerdd is Linux-only. This stub allows the rest of the
// codebase to reference the package types on non-Linux platforms (e.g. macOS
// dev builds) without compilation errors.
//
//revive:disable:package-directory-mismatch
package edgeletcontainerdd

import (
	"errors"
	"time"
)

// Service is a no-op placeholder on non-Linux platforms.
type Service struct{}

// NewService returns a stub service on non-Linux platforms.
func NewService() *Service { return &Service{} }

// Start always returns an error on non-Linux platforms.
func (s *Service) Start() error {
	return errors.New("embedded containerd is only supported on Linux")
}

// Stop is a no-op on non-Linux platforms.
func (s *Service) Stop() {}

// SetUnexpectedExitHandler is a no-op on non-Linux platforms.
func (s *Service) SetUnexpectedExitHandler(_ func(error)) {}

// StopGraceful always returns an error on non-Linux platforms.
func (s *Service) StopGraceful() error {
	return errors.New("embedded containerd is only supported on Linux")
}

// StopForce always returns an error on non-Linux platforms.
func (s *Service) StopForce() error {
	return errors.New("embedded containerd is only supported on Linux")
}

// Reconfigure always returns an error on non-Linux platforms.
func (s *Service) Reconfigure() error {
	return errors.New("embedded containerd is only supported on Linux")
}

// Reap is a no-op on non-Linux platforms.
func (s *Service) Reap() error { return nil }

// WaitReady always returns an error on non-Linux platforms.
func (s *Service) WaitReady(_ time.Duration) error {
	return errors.New("embedded containerd is only supported on Linux")
}

// Ready returns a closed channel (immediately "ready") so callers do not block.
func (s *Service) Ready() <-chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}

// IsHealthy always returns false on non-Linux platforms.
func (s *Service) IsHealthy() bool { return false }

// CleanupRuntimeArtifacts is a no-op on non-Linux platforms.
func CleanupRuntimeArtifacts() error { return nil }

// MaybeRunChildProcess is a no-op on non-Linux platforms.
func MaybeRunChildProcess(_ []string) (bool, error) { return false, nil }
