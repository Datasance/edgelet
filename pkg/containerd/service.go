//go:build linux

// Package containerd provides an in-process containerd service for iofog-agentd.
// When containerEngine=iofog, the agent runs containerd inside the same process
// (as goroutines) rather than depending on a system containerd daemon.
//
// This follows the same pattern used by k3s and kubesolo:
//   - github.com/k3s-io/k3s uses containerd in-process
//   - github.com/portainer/kubesolo uses containerd in-process
//
// Build constraint: only compiled when CGO_ENABLED=1 (required for containerd)
// and on Linux targets.

package iofogcontainerd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/cmd/containerd/command"
	"github.com/eclipse-iofog/agent/internal/constants"
	"github.com/eclipse-iofog/agent/internal/utils/logging"
)

const maxRetries = 30

const (
	containerdStartupTimeout      = 60 * time.Second
	containerdClientRetryInterval = 2 * time.Second
)

var containerdShutdownWaitTimeout = 15 * time.Second

var logger = logging.NewModuleLogger("Containerd")

// Service manages the embedded containerd process lifecycle.
type Service struct {
	runWG    sync.WaitGroup
	ctx      context.Context
	cancel   context.CancelFunc
	ready    chan struct{}
	readyOnce sync.Once
	done     chan struct{}
	runFn    func() error
}

// NewService creates an uninitialised containerd service.
// Call Run() to start containerd in-process.
func NewService() *Service {
	ctx, cancel := context.WithCancel(context.Background())
	svc := &Service{
		ctx:      ctx,
		cancel:   cancel,
		ready:    make(chan struct{}),
		done:     make(chan struct{}),
	}
	svc.runFn = svc.Run
	return svc
}

// Run starts containerd in-process, writes the config, and signals readiness.
// Blocks until containerd exits or the context is cancelled.
func (s *Service) Run() error {
	defer close(s.done)

	logger.Infof("Starting embedded containerd (socket=%s config=%s)",
		constants.IofogContainerdSocket, constants.IofogContainerdConfigFile)

	if err := s.prepare(); err != nil {
		s.cancel()
		return fmt.Errorf("containerd preparation failed: %w", err)
	}

	if err := writeConfigFile(); err != nil {
		s.cancel()
		return fmt.Errorf("failed to write containerd config: %w", err)
	}

	app := command.App()
	app.Flags = buildFlags()

	// Start containerd as a goroutine — it blocks internally.
	appErrCh := make(chan error, 1)
	s.runWG.Add(1)
	go func() {
		defer s.runWG.Done()
		if err := app.Run(nil); err != nil {
			appErrCh <- err
			return
		}
		appErrCh <- nil
	}()

	// Post-setup: health check + namespace creation.
	startupErrCh := make(chan error, 1)
	s.runWG.Add(1)
	go func() {
		defer s.runWG.Done()
		startupErrCh <- s.postSetup()
	}()

	var runErr error
	startupComplete := false

loop:
	for {
		select {
		case err := <-startupErrCh:
			startupComplete = true
			startupErrCh = nil
			if err != nil {
				logger.Errorf("Embedded containerd post-setup failed: %v", err)
				runErr = fmt.Errorf("containerd post-setup failed: %w", err)
				s.cancel()
			}
		case err := <-appErrCh:
			if err != nil {
				logger.Errorf("containerd exited with error: %v", err)
				if runErr == nil {
					runErr = fmt.Errorf("containerd exited with error: %w", err)
				}
			} else if !startupComplete && runErr == nil {
				runErr = fmt.Errorf("containerd exited before becoming ready")
			}
			s.cancel()
		case <-s.ctx.Done():
			break loop
		}
	}

	<-s.ctx.Done()
	logger.Info("Containerd service stopping.")
	if !waitForWaitGroup(&s.runWG, containerdShutdownWaitTimeout) {
		if runErr == nil {
			runErr = fmt.Errorf("containerd shutdown timed out after %s", containerdShutdownWaitTimeout)
		}
		logger.Warnf("Timed out waiting for embedded containerd goroutines to stop after %s", containerdShutdownWaitTimeout)
	}

	return runErr
}

// Ready returns a channel that is closed when containerd is ready to accept connections.
func (s *Service) Ready() <-chan struct{} {
	return s.ready
}

// Stop signals containerd to shut down and waits for all goroutines to finish.
func (s *Service) Stop() {
	s.cancel()
	if !waitForDone(s.done, containerdShutdownWaitTimeout) {
		logger.Warnf("Timed out waiting for containerd service stop after %s", containerdShutdownWaitTimeout)
	}
}

// Start launches the containerd service in a background goroutine and waits
// for it to become ready (or for the context to be cancelled).
func (s *Service) Start() error {
	errCh := make(chan error, 1)
	run := s.runFn
	if run == nil {
		run = s.Run
	}
	go func() {
		errCh <- run()
	}()

	select {
	case <-s.ready:
		return nil
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("containerd failed to start: %w", err)
		}
		return fmt.Errorf("containerd exited before becoming ready")
	}
}

// IsHealthy returns true if the containerd socket is reachable.
func (s *Service) IsHealthy() bool {
	c, err := client.New(constants.IofogContainerdSocket)
	if err != nil {
		return false
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err = c.Version(ctx)
	return err == nil
}

// prepare ensures required directories exist and removes any stale socket
// from a previous run (e.g. after crash) so containerd can bind successfully.
func (s *Service) prepare() error {
	// Remove stale socket before creating directories
	_ = os.Remove(constants.IofogContainerdSocket)

	for _, dir := range []string{
		constants.IofogContainerdRootDir,
		constants.IofogContainerdStateDir,
		constants.IofogRunDir,
	} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("mkdir %s: %w", dir, err)
		}
	}
	return nil
}

// postSetup waits for containerd health, then creates the iofog namespace.
func (s *Service) postSetup() error {
	ctx, cancel := context.WithTimeout(s.ctx, containerdStartupTimeout)
	defer cancel()

	c, err := s.waitForClient(ctx)
	if err != nil {
		return fmt.Errorf("create containerd client: %w", err)
	}
	defer c.Close()

	if err := s.healthCheck(ctx, c); err != nil {
		return fmt.Errorf("health check: %w", err)
	}

	if err := ensureNamespace(ctx, c); err != nil {
		return fmt.Errorf("ensure namespace: %w", err)
	}

	logger.Info("Embedded containerd is ready.")
	s.readyOnce.Do(func() {
		close(s.ready)
	})
	return nil
}

// healthCheck retries a Version call until containerd responds or times out.
func (s *Service) healthCheck(ctx context.Context, c *client.Client) error {
	var lastErr error
	for i := 0; i < maxRetries; i++ {
		if _, err := c.Version(ctx); err == nil {
			return nil
		} else {
			lastErr = err
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("health check timed out: %w", lastErr)
		case <-time.After(2 * time.Second):
		}
	}
	if lastErr != nil {
		return fmt.Errorf("containerd did not become healthy after %d retries: %w", maxRetries, lastErr)
	}
	return fmt.Errorf("containerd did not become healthy after %d retries", maxRetries)
}

func (s *Service) waitForClient(ctx context.Context) (*client.Client, error) {
	var lastErr error
	for {
		c, err := client.New(constants.IofogContainerdSocket)
		if err == nil {
			return c, nil
		}
		lastErr = err
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("timed out waiting for containerd socket %s: %w", constants.IofogContainerdSocket, lastErr)
		case <-time.After(containerdClientRetryInterval):
		}
	}
}

func waitForWaitGroup(wg *sync.WaitGroup, timeout time.Duration) bool {
	done := make(chan struct{})
	go func() {
		defer close(done)
		wg.Wait()
	}()
	return waitForDone(done, timeout)
}

func waitForDone(done <-chan struct{}, timeout time.Duration) bool {
	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
}

// CleanupRuntimeArtifacts removes stale runtime artifacts used by embedded containerd.
// It intentionally preserves persistent image/layer data under IofogContainerdRootDir.
func CleanupRuntimeArtifacts() error {
	paths := []string{
		constants.IofogContainerdSocket,
		constants.IofogRunDir,
		constants.IofogContainerdStateDir,
	}

	var errs []string
	for _, path := range paths {
		if err := os.RemoveAll(path); err != nil && !os.IsNotExist(err) {
			errs = append(errs, fmt.Sprintf("%s: %v", path, err))
		}
	}

	for _, dir := range []string{constants.IofogRunDir, constants.IofogContainerdStateDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			errs = append(errs, fmt.Sprintf("mkdir %s: %v", dir, err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("cleanup runtime artifacts failed: %s", strings.Join(errs, "; "))
	}
	return nil
}
