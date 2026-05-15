//go:build linux

package iofogcontainerd

import (
	"context"
	"errors"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

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
