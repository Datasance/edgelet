//go:build linux && !cgo

// Package edgeletcontainerdd provides thin-linux stubs without embedded containerd in-process.
package edgeletcontainerdd

import (
	"fmt"
	"time"
)

const containerdChildArg = "--edgelet-containerd-child"

// Service is a no-op placeholder on thin linux builds.
type Service struct{}

// NewService returns a stub service on thin linux builds.
func NewService() *Service { return &Service{} }

// Start always returns an error on thin linux builds.
func (s *Service) Start() error {
	return fmt.Errorf("embedded containerd is only available in the fat linux runtime")
}

// Stop is a no-op on thin linux builds.
func (s *Service) Stop() {}

// SetUnexpectedExitHandler is a no-op on thin linux builds.
func (s *Service) SetUnexpectedExitHandler(_ func(error)) {}

// StopGraceful always returns an error on thin linux builds.
func (s *Service) StopGraceful() error {
	return fmt.Errorf("embedded containerd is only available in the fat linux runtime")
}

// StopForce always returns an error on thin linux builds.
func (s *Service) StopForce() error {
	return fmt.Errorf("embedded containerd is only available in the fat linux runtime")
}

// Reconfigure always returns an error on thin linux builds.
func (s *Service) Reconfigure() error {
	return fmt.Errorf("embedded containerd is only available in the fat linux runtime")
}

// Reap is a no-op on thin linux builds.
func (s *Service) Reap() error { return nil }

// WaitReady always returns an error on thin linux builds.
func (s *Service) WaitReady(_ time.Duration) error {
	return fmt.Errorf("embedded containerd is only available in the fat linux runtime")
}

// Ready returns a closed channel on thin linux builds.
func (s *Service) Ready() <-chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}

// IsHealthy always returns false on thin linux builds.
func (s *Service) IsHealthy() bool { return false }

// CleanupRuntimeArtifacts is a no-op on thin linux builds.
func CleanupRuntimeArtifacts() error { return nil }

// StopOrphanedEmbeddedContainerd stops a leftover embedded containerd child after switching to docker/podman.
func StopOrphanedEmbeddedContainerd() error {
	return stopOrphanedEmbeddedContainerdFromProc()
}

// MaybeRunChildProcess rejects child mode on thin linux builds.
func MaybeRunChildProcess(args []string) (bool, error) {
	if len(args) >= 2 && args[1] == containerdChildArg {
		return true, fmt.Errorf("embedded containerd child requires the fat linux runtime")
	}
	return false, nil
}
