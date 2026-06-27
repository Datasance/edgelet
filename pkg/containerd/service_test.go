//go:build linux && cgo

package containerd

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

func TestDescribeStartupFailureNilReturnsExitedEarly(t *testing.T) {
	err := describeStartupFailure(nil)
	if !errors.Is(err, ErrContainerdExitedEarly) {
		t.Fatalf("expected ErrContainerdExitedEarly, got: %v", err)
	}
	if strings.Contains(err.Error(), "%!w(<nil>)") {
		t.Fatalf("unexpected nil wrap artifact: %v", err)
	}
}

func TestStartPropagatesRunError(t *testing.T) {
	svc := NewService()
	svc.runFn = func() error {
		return errors.New("synthetic startup failure")
	}

	start := time.Now()
	err := svc.Start()
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected startup error, got nil")
	}
	if !strings.Contains(err.Error(), "synthetic startup failure") {
		t.Fatalf("expected wrapped startup failure, got: %v", err)
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("Start should fail promptly, took %s", elapsed)
	}
}

func TestWaitReadyTimesOut(t *testing.T) {
	svc := NewService()
	svc.done = make(chan struct{})

	err := svc.WaitReady(25 * time.Millisecond)
	if err == nil {
		t.Fatal("expected readiness timeout, got nil")
	}
	if !errors.Is(err, ErrContainerdReadiness) {
		t.Fatalf("expected readiness error class, got: %v", err)
	}
}

func TestStopGracefulIsBoundedWhenDoneNeverCloses(t *testing.T) {
	svc := NewService()
	svc.done = make(chan struct{}) // keep open to emulate stuck shutdown
	svc.ctx, svc.cancel = context.WithCancel(context.Background())

	prev := containerdShutdownWaitTimeout
	containerdShutdownWaitTimeout = 25 * time.Millisecond
	defer func() { containerdShutdownWaitTimeout = prev }()

	start := time.Now()
	err := svc.StopGraceful()
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected bounded timeout error, got nil")
	}
	if !errors.Is(err, ErrContainerdStopTimeout) {
		t.Fatalf("expected stop-timeout class, got: %v", err)
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("StopGraceful should be bounded by timeout, took %s", elapsed)
	}
}

func TestStopForceIsBoundedWhenDoneNeverCloses(t *testing.T) {
	svc := NewService()
	svc.done = make(chan struct{}) // keep open to emulate stuck shutdown
	svc.ctx, svc.cancel = context.WithCancel(context.Background())

	prev := containerdShutdownWaitTimeout
	containerdShutdownWaitTimeout = 25 * time.Millisecond
	defer func() { containerdShutdownWaitTimeout = prev }()

	start := time.Now()
	err := svc.StopForce()
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected bounded timeout error, got nil")
	}
	if !errors.Is(err, ErrContainerdStopTimeout) {
		t.Fatalf("expected stop-timeout class, got: %v", err)
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("StopForce should be bounded by timeout, took %s", elapsed)
	}
}

func TestReapTimesOutWhenDoneNeverCloses(t *testing.T) {
	svc := NewService()
	svc.done = make(chan struct{}) // keep open to emulate stuck shutdown

	prev := containerdShutdownWaitTimeout
	containerdShutdownWaitTimeout = 25 * time.Millisecond
	defer func() { containerdShutdownWaitTimeout = prev }()

	start := time.Now()
	err := svc.Reap()
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected reap timeout, got nil")
	}
	if !errors.Is(err, ErrContainerdStopTimeout) {
		t.Fatalf("expected stop-timeout class, got: %v", err)
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("Reap should be bounded by timeout, took %s", elapsed)
	}
}

func TestStopGracefulReturnsNilWhenServiceDone(t *testing.T) {
	svc := NewService()
	svc.ctx, svc.cancel = context.WithCancel(context.Background())
	svc.done = make(chan struct{})

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		time.Sleep(10 * time.Millisecond)
		close(svc.done)
	}()

	prev := containerdShutdownWaitTimeout
	containerdShutdownWaitTimeout = 100 * time.Millisecond
	defer func() { containerdShutdownWaitTimeout = prev }()

	start := time.Now()
	err := svc.StopGraceful()
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("expected nil error when done closes, got: %v", err)
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("StopGraceful should complete quickly, took %s", elapsed)
	}

	wg.Wait()
}

func TestReapManagedShimsEscalatesToSigkill(t *testing.T) {
	svc := NewService()

	prevGrace := containerdShimReapGraceTimeout
	prevForce := containerdShimReapForceTimeout
	prevPoll := containerdShimReapPollInterval
	prevFinder := findManagedShimPIDs
	prevSignal := signalPID
	containerdShimReapGraceTimeout = 30 * time.Millisecond
	containerdShimReapForceTimeout = 30 * time.Millisecond
	containerdShimReapPollInterval = 5 * time.Millisecond
	defer func() {
		containerdShimReapGraceTimeout = prevGrace
		containerdShimReapForceTimeout = prevForce
		containerdShimReapPollInterval = prevPoll
		findManagedShimPIDs = prevFinder
		signalPID = prevSignal
	}()

	var mu sync.Mutex
	stage := "term"
	findManagedShimPIDs = func(_ string) ([]int, error) {
		mu.Lock()
		defer mu.Unlock()
		if stage == "term" {
			return []int{1234}, nil
		}
		return nil, nil
	}

	signals := make([]syscall.Signal, 0, 2)
	signalPID = func(_ int, sig syscall.Signal) error {
		mu.Lock()
		defer mu.Unlock()
		signals = append(signals, sig)
		if sig == syscall.SIGKILL {
			stage = "done"
		}
		return nil
	}

	if err := svc.reapManagedShims(); err != nil {
		t.Fatalf("expected reap to succeed, got: %v", err)
	}

	if len(signals) < 2 {
		t.Fatalf("expected SIGTERM then SIGKILL escalation, got %v", signals)
	}
	if signals[0] != syscall.SIGTERM {
		t.Fatalf("expected first signal SIGTERM, got %v", signals[0])
	}
	if signals[1] != syscall.SIGKILL {
		t.Fatalf("expected second signal SIGKILL, got %v", signals[1])
	}
}

func TestReapManagedShimsTimesOut(t *testing.T) {
	svc := NewService()

	prevGrace := containerdShimReapGraceTimeout
	prevForce := containerdShimReapForceTimeout
	prevPoll := containerdShimReapPollInterval
	prevFinder := findManagedShimPIDs
	prevSignal := signalPID
	containerdShimReapGraceTimeout = 20 * time.Millisecond
	containerdShimReapForceTimeout = 20 * time.Millisecond
	containerdShimReapPollInterval = 5 * time.Millisecond
	defer func() {
		containerdShimReapGraceTimeout = prevGrace
		containerdShimReapForceTimeout = prevForce
		containerdShimReapPollInterval = prevPoll
		findManagedShimPIDs = prevFinder
		signalPID = prevSignal
	}()

	findManagedShimPIDs = func(_ string) ([]int, error) {
		return []int{2222}, nil
	}
	signalPID = func(_ int, _ syscall.Signal) error {
		return nil
	}

	err := svc.reapManagedShims()
	if err == nil {
		t.Fatal("expected timeout error while shim remains alive")
	}
	if !strings.Contains(err.Error(), "still running") {
		t.Fatalf("expected still-running error, got: %v", err)
	}
}

func TestReconfigureRestartsService(t *testing.T) {
	svc := NewService()
	svc.ctx, svc.cancel = context.WithCancel(context.Background())
	svc.done = make(chan struct{})
	close(svc.done) // emulate already stopped process for StopGraceful path

	prevWriteConfig := writeConfigForService
	prevFinder := findManagedShimPIDs
	prevReadLKG := readLKGForService
	prevWriteAtomic := writeAtomicForService
	prevSignalSelf := signalSelfForService
	prevRetryDelay := containerdReconfigureRetryDelay
	prevMaxAttempts := containerdReconfigureMaxAttempts
	prevStabilityWindow := containerdReconfigureStabilityWindow
	prevShutdownWait := containerdShutdownWaitTimeout
	t.Cleanup(func() {
		writeConfigForService = prevWriteConfig
		findManagedShimPIDs = prevFinder
		readLKGForService = prevReadLKG
		writeAtomicForService = prevWriteAtomic
		signalSelfForService = prevSignalSelf
		containerdReconfigureRetryDelay = prevRetryDelay
		containerdReconfigureMaxAttempts = prevMaxAttempts
		containerdReconfigureStabilityWindow = prevStabilityWindow
		containerdShutdownWaitTimeout = prevShutdownWait
	})
	writeConfigForService = func() error { return nil }
	findManagedShimPIDs = func(_ string) ([]int, error) { return nil, nil }
	readLKGForService = func(_ string) ([]byte, error) { return nil, errors.New("missing lkg") }
	writeAtomicForService = func(_ string, _ []byte, _ os.FileMode) error { return nil }
	signalSelfForService = func(_ syscall.Signal) error { return nil }
	containerdReconfigureRetryDelay = 1 * time.Millisecond
	containerdReconfigureMaxAttempts = 1
	containerdReconfigureStabilityWindow = 1 * time.Millisecond
	containerdShutdownWaitTimeout = 1 * time.Millisecond

	svc.runFn = func() error {
		svc.readyOnce.Do(func() { close(svc.ready) })
		<-svc.ctx.Done()
		close(svc.done)
		return nil
	}

	if err := svc.Reconfigure(); err != nil {
		t.Fatalf("expected reconfigure success, got: %v", err)
	}
}

func TestReconfigureReturnsDeterministicRestartError(t *testing.T) {
	svc := NewService()
	svc.ctx, svc.cancel = context.WithCancel(context.Background())
	svc.done = make(chan struct{})
	close(svc.done) // emulate already stopped process for StopGraceful path

	prevWriteConfig := writeConfigForService
	prevFinder := findManagedShimPIDs
	prevReadLKG := readLKGForService
	prevWriteAtomic := writeAtomicForService
	prevSignalSelf := signalSelfForService
	prevRetryDelay := containerdReconfigureRetryDelay
	prevMaxAttempts := containerdReconfigureMaxAttempts
	prevStabilityWindow := containerdReconfigureStabilityWindow
	prevShutdownWait := containerdShutdownWaitTimeout
	t.Cleanup(func() {
		writeConfigForService = prevWriteConfig
		findManagedShimPIDs = prevFinder
		readLKGForService = prevReadLKG
		writeAtomicForService = prevWriteAtomic
		signalSelfForService = prevSignalSelf
		containerdReconfigureRetryDelay = prevRetryDelay
		containerdReconfigureMaxAttempts = prevMaxAttempts
		containerdReconfigureStabilityWindow = prevStabilityWindow
		containerdShutdownWaitTimeout = prevShutdownWait
	})
	writeConfigForService = func() error { return nil }
	findManagedShimPIDs = func(_ string) ([]int, error) { return nil, nil }
	readLKGForService = func(_ string) ([]byte, error) { return nil, errors.New("missing lkg") }
	writeAtomicForService = func(_ string, _ []byte, _ os.FileMode) error { return nil }
	signaled := false
	signalSelfForService = func(_ syscall.Signal) error {
		signaled = true
		return nil
	}
	containerdReconfigureRetryDelay = 1 * time.Millisecond
	containerdReconfigureMaxAttempts = 1
	containerdReconfigureStabilityWindow = 1 * time.Millisecond
	containerdShutdownWaitTimeout = 1 * time.Millisecond

	svc.runFn = func() error {
		return errors.New("synthetic restart failure")
	}

	err := svc.Reconfigure()
	if err == nil {
		t.Fatal("expected reconfigure error")
	}
	if !errors.Is(err, ErrContainerdReconfigure) {
		t.Fatalf("expected ErrContainerdReconfigure, got: %v", err)
	}
	if !signaled {
		t.Fatal("expected daemon restart escalation signal on persistent failure")
	}
}

func TestReconfigureRollbackPathWritesLKGAndSkipsEscalationOnRecoveredRuntime(t *testing.T) {
	svc := NewService()
	svc.ctx, svc.cancel = context.WithCancel(context.Background())
	svc.done = make(chan struct{})
	close(svc.done)

	prevWriteConfig := writeConfigForService
	prevFinder := findManagedShimPIDs
	prevReadLKG := readLKGForService
	prevWriteAtomic := writeAtomicForService
	prevSignalSelf := signalSelfForService
	prevRetryDelay := containerdReconfigureRetryDelay
	prevMaxAttempts := containerdReconfigureMaxAttempts
	prevStabilityWindow := containerdReconfigureStabilityWindow
	prevShutdownWait := containerdShutdownWaitTimeout
	t.Cleanup(func() {
		writeConfigForService = prevWriteConfig
		findManagedShimPIDs = prevFinder
		readLKGForService = prevReadLKG
		writeAtomicForService = prevWriteAtomic
		signalSelfForService = prevSignalSelf
		containerdReconfigureRetryDelay = prevRetryDelay
		containerdReconfigureMaxAttempts = prevMaxAttempts
		containerdReconfigureStabilityWindow = prevStabilityWindow
		containerdShutdownWaitTimeout = prevShutdownWait
	})

	writeConfigForService = func() error { return nil }
	findManagedShimPIDs = func(_ string) ([]int, error) { return nil, nil }
	readLKGForService = func(_ string) ([]byte, error) { return []byte("version = 3\n"), nil }
	wroteRollback := false
	writeAtomicForService = func(path string, content []byte, _ os.FileMode) error {
		if strings.HasSuffix(path, "/config.toml") && bytes.Equal(content, []byte("version = 3\n")) {
			wroteRollback = true
		}
		return nil
	}
	signaled := false
	signalSelfForService = func(_ syscall.Signal) error {
		signaled = true
		return nil
	}
	containerdReconfigureRetryDelay = 1 * time.Millisecond
	containerdReconfigureMaxAttempts = 1
	containerdReconfigureStabilityWindow = 1 * time.Millisecond
	containerdShutdownWaitTimeout = 1 * time.Millisecond

	firstStart := true
	svc.runFn = func() error {
		if firstStart {
			firstStart = false
			close(svc.done)
			return errors.New("synthetic start failure")
		}
		svc.readyOnce.Do(func() { close(svc.ready) })
		<-svc.ctx.Done()
		close(svc.done)
		return nil
	}

	err := svc.Reconfigure()
	if err == nil {
		t.Fatal("expected reconfigure to report failure even after rollback recovery")
	}
	if !errors.Is(err, ErrContainerdReconfigure) {
		t.Fatalf("expected ErrContainerdReconfigure, got: %v", err)
	}
	if !wroteRollback {
		t.Fatal("expected rollback to write LKG config")
	}
	if signaled {
		t.Fatal("did not expect escalation signal when rollback restored runtime")
	}
}

func TestNotifyUnexpectedExitInvokesHandler(t *testing.T) {
	svc := NewService()
	called := make(chan error, 1)
	svc.SetUnexpectedExitHandler(func(err error) {
		called <- err
	})

	svc.notifyUnexpectedExit(errors.New("synthetic unexpected exit"))

	select {
	case err := <-called:
		if err == nil || !strings.Contains(err.Error(), "synthetic unexpected exit") {
			t.Fatalf("unexpected callback error payload: %v", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected unexpected-exit handler callback")
	}
}

func TestSpawnChildSurfacesRuntimeResolutionError(t *testing.T) {
	prev := resolveChildExecutable
	resolveChildExecutable = func() (string, error) {
		return "", errors.New("missing fat runtime")
	}
	t.Cleanup(func() { resolveChildExecutable = prev })

	svc := NewService()
	_, err := svc.spawnChild()
	if err == nil || !strings.Contains(err.Error(), "resolve fat runtime") {
		t.Fatalf("expected resolution error, got %v", err)
	}
}

func withTempCleanupDirs(t *testing.T) (stateDir, runDir string) {
	stateDir = t.TempDir()
	runDir = t.TempDir()

	prevState := containerdStateDirForCleanup
	prevArtifactState := runtimeArtifactCleanupStateDir
	prevRun := runtimeArtifactCleanupRunDir
	prevSocket := runtimeArtifactCleanupSocket
	containerdStateDirForCleanup = stateDir
	runtimeArtifactCleanupStateDir = stateDir
	runtimeArtifactCleanupRunDir = runDir
	runtimeArtifactCleanupSocket = filepath.Join(runDir, "containerd.sock")
	t.Cleanup(func() {
		containerdStateDirForCleanup = prevState
		runtimeArtifactCleanupStateDir = prevArtifactState
		runtimeArtifactCleanupRunDir = prevRun
		runtimeArtifactCleanupSocket = prevSocket
	})
	return stateDir, runDir
}

func TestCleanupStaleRuntimeTasks_RemovesMissingAddress(t *testing.T) {
	stateDir, _ := withTempCleanupDirs(t)
	taskBase := filepath.Join(stateDir, runtimeV2TaskDirName)
	staleTask := filepath.Join(taskBase, "stale-task-id")
	if err := os.MkdirAll(staleTask, 0o755); err != nil {
		t.Fatalf("mkdir stale task: %v", err)
	}

	if err := CleanupStaleRuntimeTasks(); err != nil {
		t.Fatalf("cleanup failed: %v", err)
	}
	if _, err := os.Stat(staleTask); !os.IsNotExist(err) {
		t.Fatalf("expected stale task dir removed, stat err=%v", err)
	}
}

func TestCleanupStaleRuntimeTasks_PreservesValidTask(t *testing.T) {
	stateDir, _ := withTempCleanupDirs(t)
	taskBase := filepath.Join(stateDir, runtimeV2TaskDirName)
	validTask := filepath.Join(taskBase, "valid-task-id")
	if err := os.MkdirAll(validTask, 0o755); err != nil {
		t.Fatalf("mkdir valid task: %v", err)
	}
	if err := os.WriteFile(filepath.Join(validTask, "address"), []byte("unix:///run/shim.sock"), 0o600); err != nil {
		t.Fatalf("write address file: %v", err)
	}

	if err := CleanupStaleRuntimeTasks(); err != nil {
		t.Fatalf("cleanup failed: %v", err)
	}
	if _, err := os.Stat(validTask); err != nil {
		t.Fatalf("expected valid task dir preserved, stat err=%v", err)
	}
}

func TestCleanupStaleRuntimeTasks_PreservesRootDir(t *testing.T) {
	stateDir, _ := withTempCleanupDirs(t)
	rootDir := t.TempDir()
	rootMarker := filepath.Join(rootDir, "image-cache-marker")
	if err := os.WriteFile(rootMarker, []byte("keep"), 0o600); err != nil {
		t.Fatalf("write root marker: %v", err)
	}

	taskBase := filepath.Join(stateDir, runtimeV2TaskDirName)
	staleTask := filepath.Join(taskBase, "orphan-task")
	if err := os.MkdirAll(staleTask, 0o755); err != nil {
		t.Fatalf("mkdir stale task: %v", err)
	}

	if err := CleanupStaleRuntimeTasks(); err != nil {
		t.Fatalf("cleanup failed: %v", err)
	}
	if _, err := os.Stat(rootMarker); err != nil {
		t.Fatalf("expected root dir untouched, stat err=%v", err)
	}
}

func TestCleanupRuntimeArtifacts_StillOnRetry(t *testing.T) {
	stateDir, runDir := withTempCleanupDirs(t)
	stateMarker := filepath.Join(stateDir, "stale-state-marker")
	if err := os.WriteFile(stateMarker, []byte("stale"), 0o600); err != nil {
		t.Fatalf("write state marker: %v", err)
	}

	if err := CleanupRuntimeArtifacts(); err != nil {
		t.Fatalf("artifact cleanup failed: %v", err)
	}
	if _, err := os.Stat(stateMarker); !os.IsNotExist(err) {
		t.Fatalf("expected state marker removed, stat err=%v", err)
	}
	if _, err := os.Stat(stateDir); err != nil {
		t.Fatalf("expected state dir recreated, stat err=%v", err)
	}
	if _, err := os.Stat(runDir); err != nil {
		t.Fatalf("expected run dir recreated, stat err=%v", err)
	}
}
