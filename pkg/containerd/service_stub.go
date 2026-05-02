//go:build !linux

// Package iofogcontainerd is Linux-only. This stub allows the rest of the
// codebase to reference the package types on non-Linux platforms (e.g. macOS
// dev builds) without compilation errors.
package iofogcontainerd

import "fmt"

// Service is a no-op placeholder on non-Linux platforms.
type Service struct{}

// NewService returns a stub service on non-Linux platforms.
func NewService() *Service { return &Service{} }

// Start always returns an error on non-Linux platforms.
func (s *Service) Start() error {
	return fmt.Errorf("embedded containerd is only supported on Linux")
}

// Stop is a no-op on non-Linux platforms.
func (s *Service) Stop() {}

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
