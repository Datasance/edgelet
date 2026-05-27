//go:build linux && cgo

package edgeletcontainerdd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/cmd/containerd/command"
	"github.com/datasance/edgelet/internal/constants"
	"github.com/datasance/edgelet/internal/utils/logging"
	"github.com/datasance/edgelet/pkg/data"
)

const maxRetries = 30

const (
	containerdStartupTimeout      = 60 * time.Second
	containerdClientRetryInterval = 2 * time.Second
	containerdChildArg            = "--edgelet-containerd-child"
)

const (
	reconfigureStageWriteConfig     = "write_config"
	reconfigureStageStopRuntime     = "stop_runtime"
	reconfigureStageStartRuntime    = "start_runtime"
	reconfigureStageWaitCRIReady    = "wait_cri_ready"
	reconfigureStageVerifyStability = "verify_stability"
	reconfigureStageRollbackConfig  = "rollback_config"
	reconfigureStageEscalateRestart = "escalate_restart"
	reconfigureStageDone            = "done"
)

var containerdShutdownWaitTimeout = 15 * time.Second
var containerdShimReapGraceTimeout = 5 * time.Second
var containerdShimReapForceTimeout = 5 * time.Second
var containerdShimReapPollInterval = 100 * time.Millisecond
var containerdReconfigureStabilityWindow = 3 * time.Second
var containerdReconfigureRetryDelay = 500 * time.Millisecond
var containerdReconfigureMaxAttempts = 2

var findManagedShimPIDs = findManagedShimPIDsFromProc
var resolveChildExecutable = data.RuntimeBinary
var signalPID = syscall.Kill
var writeConfigForService = writeConfigFile
var readLKGForService = readLastKnownGoodConfig
var writeAtomicForService = writeFileAtomically
var signalSelfForService = func(sig syscall.Signal) error { return syscall.Kill(os.Getpid(), sig) }
var sleepForReconfigure = time.Sleep

var (
	ErrContainerdSpawnFailure   = errors.New("containerd spawn failure")
	ErrContainerdReadiness      = errors.New("containerd readiness failure")
	ErrContainerdExitedEarly    = errors.New("containerd exited before ready")
	ErrContainerdStopTimeout    = errors.New("containerd stop timeout")
	ErrContainerdUnexpectedExit = errors.New("containerd unexpected exit")
	ErrContainerdReconfigure    = errors.New("containerd reconfigure failure")
)

var logger = logging.NewModuleLogger("Containerd")

// Service manages the embedded containerd child-process lifecycle.
type Service struct {
	mu sync.Mutex

	ctx    context.Context
	cancel context.CancelFunc

	cmd     *exec.Cmd
	waitErr error

	ready     chan struct{}
	readyOnce sync.Once
	done      chan struct{}

	runFn func() error

	unexpectedExitHandler func(error)
}

type ErrContainerdReconfigureOperation struct {
	Stage   string
	Attempt int
	Err     error
}

func (e *ErrContainerdReconfigureOperation) Error() string {
	if e == nil || e.Err == nil {
		return "containerd reconfigure operation failed"
	}
	return e.Err.Error()
}

func (e *ErrContainerdReconfigureOperation) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func (e *ErrContainerdReconfigureOperation) Details() map[string]interface{} {
	stage := strings.TrimSpace(strings.ToLower(e.Stage))
	if stage == "" {
		return nil
	}
	details := map[string]interface{}{
		"stage": stage,
	}
	if e.Attempt > 0 {
		details["attempt"] = e.Attempt
	}
	return details
}

// NewService creates an uninitialized containerd service.
func NewService() *Service {
	ctx, cancel := context.WithCancel(context.Background())
	svc := &Service{
		ctx:    ctx,
		cancel: cancel,
		ready:  make(chan struct{}),
		done:   make(chan struct{}),
	}
	svc.runFn = svc.Run
	return svc
}

// MaybeRunChildProcess runs containerd in child mode when the daemon is invoked
// with the dedicated internal child argument.
func MaybeRunChildProcess(args []string) (bool, error) {
	if len(args) < 2 || args[1] != containerdChildArg {
		return false, nil
	}
	return true, runContainerdChild(args[2:])
}

func runContainerdChild(args []string) error {
	app := command.App()
	app.Flags = buildFlags()
	argv := append([]string{"containerd-child"}, args...)
	return app.Run(argv)
}

// Run starts containerd as a managed child process and blocks until it exits.
func (s *Service) Run() error {
	defer close(s.done)

	logger.Infof("Starting embedded containerd child process (socket=%s config=%s)",
		constants.EdgeletContainerdSocket, constants.EdgeletContainerdConfigFile)

	if err := s.prepare(); err != nil {
		return fmt.Errorf("%w: prepare runtime directories: %v", ErrContainerdSpawnFailure, err)
	}

	if err := writeConfigForService(); err != nil {
		return fmt.Errorf("%w: write config: %v", ErrContainerdSpawnFailure, err)
	}

	cmd, err := s.spawnChild()
	if err != nil {
		return fmt.Errorf("%w: %v", ErrContainerdSpawnFailure, err)
	}

	processExitCh := make(chan error, 1)
	go func() {
		processExitCh <- cmd.Wait()
	}()

	startupErrCh := make(chan error, 1)
	go func() {
		startupErrCh <- s.postSetup()
	}()

	ready := false
	var runErr error

	for {
		select {
		case err := <-startupErrCh:
			startupErrCh = nil
			if err != nil {
				runErr = fmt.Errorf("%w: %v", ErrContainerdReadiness, err)
				logger.Errorf("Embedded containerd readiness failed: %v", err)
				s.cancel()
				_ = s.signalProcess(syscall.SIGTERM)
			} else {
				ready = true
			}
		case err := <-processExitCh:
			s.mu.Lock()
			s.waitErr = err
			s.mu.Unlock()

			if runErr != nil {
				return runErr
			}
			if !ready {
				if err != nil {
					return fmt.Errorf("%w: %v", ErrContainerdExitedEarly, err)
				}
				return ErrContainerdExitedEarly
			}
			if s.ctx.Err() != nil {
				if err != nil {
					logger.Debugf("Containerd child exited during shutdown: %v", err)
				}
				return nil
			}
			if err != nil {
				s.notifyUnexpectedExit(fmt.Errorf("%w: %v", ErrContainerdUnexpectedExit, err))
				return fmt.Errorf("%w: %v", ErrContainerdUnexpectedExit, err)
			}
			s.notifyUnexpectedExit(ErrContainerdUnexpectedExit)
			return ErrContainerdUnexpectedExit
		case <-s.ctx.Done():
			logger.Info("Containerd service stopping.")
			if err := s.signalProcess(syscall.SIGTERM); err != nil {
				logger.Warnf("Failed to signal containerd child with SIGTERM: %v", err)
			}

			select {
			case err := <-processExitCh:
				s.mu.Lock()
				s.waitErr = err
				s.mu.Unlock()
				if err != nil {
					logger.Debugf("Containerd child exited after stop signal: %v", err)
				}
				return nil
			case <-time.After(containerdShutdownWaitTimeout):
				logger.Warnf("Timed out waiting for containerd child to stop after %s; forcing kill", containerdShutdownWaitTimeout)
				if killErr := s.signalProcess(syscall.SIGKILL); killErr != nil {
					logger.Warnf("Failed to force-kill containerd child: %v", killErr)
				}
				select {
				case err := <-processExitCh:
					s.mu.Lock()
					s.waitErr = err
					s.mu.Unlock()
					if err != nil {
						logger.Debugf("Containerd child exited after SIGKILL: %v", err)
					}
				case <-time.After(containerdShutdownWaitTimeout):
					return fmt.Errorf("%w: exceeded forced stop wait window", ErrContainerdStopTimeout)
				}
				return nil
			}
		}
	}
}

// Reconfigure rewrites containerd config and performs a controlled restart.
func (s *Service) Reconfigure() error {
	previousLKG, lkgErr := readLKGForService(constants.EdgeletContainerdConfigFile)
	hasLKG := lkgErr == nil && len(previousLKG) > 0

	if err := writeConfigForService(); err != nil {
		return fmt.Errorf("%w: %w", ErrContainerdReconfigure, wrapReconfigureError(reconfigureStageWriteConfig, 0, fmt.Errorf("write config: %w", err)))
	}

	var lastErr error
	for attempt := 1; attempt <= containerdReconfigureMaxAttempts; attempt++ {
		if err := s.reconfigureAttempt(attempt); err == nil {
			return nil
		} else {
			lastErr = err
			logger.Warnf("Embedded containerd reconfigure attempt %d/%d failed: %v", attempt, containerdReconfigureMaxAttempts, err)
		}
		if attempt < containerdReconfigureMaxAttempts {
			sleepForReconfigure(containerdReconfigureRetryDelay)
		}
	}

	rollbackErr := s.rollbackToLKG(previousLKG, hasLKG)
	if rollbackErr == nil {
		return fmt.Errorf(
			"%w: %w",
			ErrContainerdReconfigure,
			wrapReconfigureError(
				reconfigureStageRollbackConfig,
				0,
				fmt.Errorf("reconfigure failed and rollback to last-known-good config succeeded: %v", lastErr),
			),
		)
	}
	lastErr = fmt.Errorf("%v; rollback failed: %w", lastErr, rollbackErr)
	if err := s.escalateRestart(lastErr); err != nil {
		return fmt.Errorf("%w: %w", ErrContainerdReconfigure, err)
	}
	return fmt.Errorf("%w: %w", ErrContainerdReconfigure, wrapReconfigureError(reconfigureStageEscalateRestart, 0, lastErr))
}

// Ready returns a channel that closes when containerd is ready to accept requests.
func (s *Service) Ready() <-chan struct{} {
	return s.ready
}

// Start launches the service in background and waits for readiness.
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
		return describeStartupFailure(err)
	}
}

func describeStartupFailure(err error) error {
	if err == nil {
		return ErrContainerdExitedEarly
	}
	switch {
	case errors.Is(err, ErrContainerdSpawnFailure):
		return fmt.Errorf("spawn embedded containerd child: %w", err)
	case errors.Is(err, ErrContainerdReadiness):
		return fmt.Errorf("wait for embedded containerd readiness: %w", err)
	case errors.Is(err, ErrContainerdExitedEarly):
		return err
	default:
		return fmt.Errorf("embedded containerd run loop failed: %w", err)
	}
}

// WaitReady waits for the service to become ready up to timeout.
func (s *Service) WaitReady(timeout time.Duration) error {
	select {
	case <-s.ready:
		return nil
	case <-s.done:
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.waitErr != nil {
			return fmt.Errorf("%w: %v", ErrContainerdExitedEarly, s.waitErr)
		}
		return ErrContainerdExitedEarly
	case <-time.After(timeout):
		return fmt.Errorf("%w: timed out after %s", ErrContainerdReadiness, timeout)
	}
}

// StopGraceful requests a graceful shutdown and waits for completion.
func (s *Service) StopGraceful() error {
	s.cancel()
	_ = s.signalProcess(syscall.SIGTERM)
	if !waitForDone(s.done, containerdShutdownWaitTimeout) {
		return fmt.Errorf("%w: graceful stop exceeded %s", ErrContainerdStopTimeout, containerdShutdownWaitTimeout)
	}
	if err := s.reapManagedShims(); err != nil {
		return err
	}
	return nil
}

// StopForce force-kills the child process and waits for completion.
func (s *Service) StopForce() error {
	s.cancel()
	_ = s.signalProcess(syscall.SIGKILL)
	if !waitForDone(s.done, containerdShutdownWaitTimeout) {
		return fmt.Errorf("%w: force stop exceeded %s", ErrContainerdStopTimeout, containerdShutdownWaitTimeout)
	}
	if err := s.reapManagedShims(); err != nil {
		return err
	}
	return nil
}

func (s *Service) resetForRestart() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.cmd = nil
	s.waitErr = nil
	s.ready = make(chan struct{})
	s.readyOnce = sync.Once{}
	s.done = make(chan struct{})
	if s.runFn == nil {
		s.runFn = s.Run
	}
}

// Reap waits for the service goroutine to finish.
func (s *Service) Reap() error {
	if !waitForDone(s.done, containerdShutdownWaitTimeout) {
		return fmt.Errorf("%w: reap exceeded %s", ErrContainerdStopTimeout, containerdShutdownWaitTimeout)
	}
	return nil
}

// Stop keeps backward-compatible behavior while enforcing escalation.
func (s *Service) Stop() {
	if err := s.StopGraceful(); err == nil {
		return
	} else {
		logger.Warnf("Graceful containerd stop failed: %v", err)
	}
	if err := s.StopForce(); err != nil {
		logger.Warnf("Forced containerd stop failed: %v", err)
	}
}

func (s *Service) SetUnexpectedExitHandler(handler func(error)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.unexpectedExitHandler = handler
}

// IsHealthy returns true if the containerd socket is reachable.
func (s *Service) IsHealthy() bool {
	c, err := client.New(constants.EdgeletContainerdSocket)
	if err != nil {
		return false
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err = c.Version(ctx)
	return err == nil
}

func (s *Service) spawnChild() (*exec.Cmd, error) {
	execPath, err := resolveChildExecutable()
	if err != nil {
		return nil, fmt.Errorf("resolve fat runtime executable: %w", err)
	}

	args := []string{
		containerdChildArg,
		"--config", constants.EdgeletContainerdConfigFile,
		"--address", constants.EdgeletContainerdSocket,
		"--root", constants.EdgeletContainerdRootDir,
		"--state", constants.EdgeletContainerdStateDir,
		"--log-level", "warn",
	}

	cmd := exec.Command(execPath, args...) // #nosec G204 -- fixed executable path + controlled args
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start child process: %w", err)
	}

	s.mu.Lock()
	s.cmd = cmd
	s.mu.Unlock()

	logger.Infof("Embedded containerd child started (pid=%d)", cmd.Process.Pid)
	return cmd, nil
}

func (s *Service) signalProcess(sig syscall.Signal) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cmd == nil || s.cmd.Process == nil {
		return nil
	}
	pid := s.cmd.Process.Pid
	if pid <= 0 {
		return nil
	}
	if err := syscall.Kill(-pid, sig); err != nil && !errors.Is(err, syscall.ESRCH) {
		return fmt.Errorf("signal process group %d with %s: %w", pid, sig, err)
	}
	return nil
}

func (s *Service) notifyUnexpectedExit(err error) {
	s.mu.Lock()
	handler := s.unexpectedExitHandler
	s.mu.Unlock()
	if handler == nil {
		return
	}
	go handler(err)
}

func wrapReconfigureError(stage string, attempt int, err error) error {
	if err == nil {
		return nil
	}
	return &ErrContainerdReconfigureOperation{
		Stage:   stage,
		Attempt: attempt,
		Err:     err,
	}
}

func (s *Service) reconfigureAttempt(attempt int) error {
	if err := s.StopGraceful(); err != nil {
		logger.Warnf("Graceful stop during reconfigure attempt %d failed: %v", attempt, err)
		if forceErr := s.StopForce(); forceErr != nil {
			return wrapReconfigureError(reconfigureStageStopRuntime, attempt, fmt.Errorf("stop for reconfigure failed (graceful: %v, force: %v)", err, forceErr))
		}
	}
	s.resetForRestart()
	if err := s.Start(); err != nil {
		return wrapReconfigureError(reconfigureStageStartRuntime, attempt, fmt.Errorf("restart failed: %w", err))
	}
	if err := s.WaitReady(containerdStartupTimeout); err != nil {
		return wrapReconfigureError(reconfigureStageWaitCRIReady, attempt, err)
	}
	if err := s.verifyStabilityWindow(containerdReconfigureStabilityWindow); err != nil {
		return wrapReconfigureError(reconfigureStageVerifyStability, attempt, err)
	}
	return nil
}

func (s *Service) verifyStabilityWindow(window time.Duration) error {
	deadline := time.Now().Add(window)
	for {
		select {
		case <-s.done:
			s.mu.Lock()
			waitErr := s.waitErr
			s.mu.Unlock()
			if waitErr != nil {
				return fmt.Errorf("containerd exited during stability window: %w", waitErr)
			}
			return fmt.Errorf("containerd exited during stability window")
		default:
		}
		if time.Now().After(deadline) {
			return nil
		}
		sleepForReconfigure(200 * time.Millisecond)
	}
}

func (s *Service) rollbackToLKG(previousLKG []byte, hasLKG bool) error {
	if !hasLKG {
		return wrapReconfigureError(reconfigureStageRollbackConfig, 0, fmt.Errorf("last-known-good config is unavailable"))
	}
	if err := writeAtomicForService(constants.EdgeletContainerdConfigFile, previousLKG, 0644); err != nil {
		return wrapReconfigureError(reconfigureStageRollbackConfig, 0, fmt.Errorf("restore last-known-good config: %w", err))
	}
	if err := s.reconfigureAttempt(0); err != nil {
		return wrapReconfigureError(reconfigureStageRollbackConfig, 0, fmt.Errorf("restart after rollback failed: %w", err))
	}
	return nil
}

func (s *Service) escalateRestart(cause error) error {
	if err := signalSelfForService(syscall.SIGTERM); err != nil {
		return wrapReconfigureError(reconfigureStageEscalateRestart, 0, fmt.Errorf("failed to signal daemon for restart (cause: %v): %w", cause, err))
	}
	return wrapReconfigureError(reconfigureStageEscalateRestart, 0, fmt.Errorf("requested daemon restart after persistent reconfigure failure: %v", cause))
}

// prepare ensures required directories exist and removes any stale socket
// from a previous run (e.g. after crash) so containerd can bind successfully.
func (s *Service) prepare() error {
	_ = os.Remove(constants.EdgeletContainerdSocket)

	for _, dir := range []string{
		constants.EdgeletContainerdRootDir,
		constants.EdgeletContainerdStateDir,
		constants.EdgeletRunDir,
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
		c, err := client.New(constants.EdgeletContainerdSocket)
		if err == nil {
			return c, nil
		}
		lastErr = err
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("timed out waiting for containerd socket %s: %w", constants.EdgeletContainerdSocket, lastErr)
		case <-time.After(containerdClientRetryInterval):
		}
	}
}

func waitForDone(done <-chan struct{}, timeout time.Duration) bool {
	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
}

func (s *Service) reapManagedShims() error {
	remaining, err := findManagedShimPIDs(constants.EdgeletContainerdSocket)
	if err != nil {
		return fmt.Errorf("discover managed shims before reap: %w", err)
	}
	if len(remaining) == 0 {
		return nil
	}

	logger.Warnf("Detected %d managed containerd shim process(es) after containerd stop; reaping", len(remaining))
	for _, pid := range remaining {
		if err := signalPID(pid, syscall.SIGTERM); err != nil && !errors.Is(err, syscall.ESRCH) {
			logger.Warnf("Failed to signal shim pid %d with SIGTERM: %v", pid, err)
		}
	}

	if exited, waitErr := waitForManagedShimsExit(constants.EdgeletContainerdSocket, containerdShimReapGraceTimeout); waitErr != nil {
		return fmt.Errorf("wait for shim SIGTERM completion: %w", waitErr)
	} else if exited {
		return nil
	}

	remaining, err = findManagedShimPIDs(constants.EdgeletContainerdSocket)
	if err != nil {
		return fmt.Errorf("discover managed shims before SIGKILL: %w", err)
	}
	for _, pid := range remaining {
		if err := signalPID(pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
			logger.Warnf("Failed to signal shim pid %d with SIGKILL: %v", pid, err)
		}
	}

	if exited, waitErr := waitForManagedShimsExit(constants.EdgeletContainerdSocket, containerdShimReapForceTimeout); waitErr != nil {
		return fmt.Errorf("wait for shim SIGKILL completion: %w", waitErr)
	} else if !exited {
		if stillRunning, listErr := findManagedShimPIDs(constants.EdgeletContainerdSocket); listErr == nil {
			return fmt.Errorf("managed shims still running after reap attempts: pids=%v", stillRunning)
		}
		return fmt.Errorf("managed shims still running after reap attempts")
	}
	return nil
}

func waitForManagedShimsExit(socketPath string, timeout time.Duration) (bool, error) {
	deadline := time.Now().Add(timeout)
	for {
		pids, err := findManagedShimPIDs(socketPath)
		if err != nil {
			return false, err
		}
		if len(pids) == 0 {
			return true, nil
		}
		if time.Now().After(deadline) {
			return false, nil
		}
		time.Sleep(containerdShimReapPollInterval)
	}
}

func findManagedShimPIDsFromProc(socketPath string) ([]int, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, fmt.Errorf("read /proc: %w", err)
	}

	pids := make([]int, 0)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, convErr := strconv.Atoi(entry.Name())
		if convErr != nil || pid <= 0 {
			continue
		}
		cmdline, readErr := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
		if readErr != nil {
			continue
		}
		if !bytes.Contains(cmdline, []byte("containerd-shim-")) {
			continue
		}
		if !bytes.Contains(cmdline, []byte(socketPath)) {
			continue
		}
		pids = append(pids, pid)
	}
	return pids, nil
}

// CleanupRuntimeArtifacts removes stale runtime artifacts used by embedded containerd.
// It intentionally preserves persistent image/layer data under EdgeletContainerdRootDir.
func CleanupRuntimeArtifacts() error {
	paths := []string{
		constants.EdgeletContainerdSocket,
		constants.EdgeletRunDir,
		constants.EdgeletContainerdStateDir,
	}

	var errs []string
	for _, path := range paths {
		if err := os.RemoveAll(path); err != nil && !os.IsNotExist(err) {
			errs = append(errs, fmt.Sprintf("%s: %v", path, err))
		}
	}

	for _, dir := range []string{constants.EdgeletRunDir, constants.EdgeletContainerdStateDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			errs = append(errs, fmt.Sprintf("mkdir %s: %v", dir, err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("cleanup runtime artifacts failed: %s", strings.Join(errs, "; "))
	}
	return nil
}
