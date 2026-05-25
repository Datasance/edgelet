//go:build linux && lite

// Package edgeletcontainerdd provides lite stubs on Linux without embedded containerd.
package edgeletcontainerdd

import (
	"fmt"
	"time"
)

const containerdChildArg = "--edgelet-containerd-child"

// Service is a no-op placeholder on lite Linux builds.
type Service struct{}

// NewService returns a stub service on lite Linux builds.
func NewService() *Service { return &Service{} }

// Start always returns an error on lite Linux builds.
func (s *Service) Start() error {
	return fmt.Errorf("embedded containerd is only supported in full linux builds")
}

// Stop is a no-op on lite Linux builds.
func (s *Service) Stop() {}

// SetUnexpectedExitHandler is a no-op on lite Linux builds.
func (s *Service) SetUnexpectedExitHandler(_ func(error)) {}

// StopGraceful always returns an error on lite Linux builds.
func (s *Service) StopGraceful() error {
	return fmt.Errorf("embedded containerd is only supported in full linux builds")
}

// StopForce always returns an error on lite Linux builds.
func (s *Service) StopForce() error {
	return fmt.Errorf("embedded containerd is only supported in full linux builds")
}

// Reconfigure always returns an error on lite Linux builds.
func (s *Service) Reconfigure() error {
	return fmt.Errorf("embedded containerd is only supported in full linux builds")
}

// Reap is a no-op on lite Linux builds.
func (s *Service) Reap() error { return nil }

// WaitReady always returns an error on lite Linux builds.
func (s *Service) WaitReady(_ time.Duration) error {
	return fmt.Errorf("embedded containerd is only supported in full linux builds")
}

// Ready returns a closed channel on lite Linux builds.
func (s *Service) Ready() <-chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}

// IsHealthy always returns false on lite Linux builds.
func (s *Service) IsHealthy() bool { return false }

// CleanupRuntimeArtifacts is a no-op on lite Linux builds.
func CleanupRuntimeArtifacts() error { return nil }

// MaybeRunChildProcess rejects child mode on lite Linux builds.
func MaybeRunChildProcess(args []string) (bool, error) {
	if len(args) >= 2 && args[1] == containerdChildArg {
		return true, fmt.Errorf("embedded containerd child requires a full linux build")
	}
	return false, nil
}
