//go:build linux

package iofogcontainerd

import (
	"errors"
	"strings"
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

func TestStopIsBoundedWhenDoneNeverCloses(t *testing.T) {
	svc := NewService()
	svc.done = make(chan struct{}) // keep open to emulate stuck shutdown

	prev := containerdShutdownWaitTimeout
	containerdShutdownWaitTimeout = 25 * time.Millisecond
	defer func() {
		containerdShutdownWaitTimeout = prev
	}()

	start := time.Now()
	svc.Stop()
	elapsed := time.Since(start)

	if elapsed > 500*time.Millisecond {
		t.Fatalf("Stop should be bounded by timeout, took %s", elapsed)
	}
}
