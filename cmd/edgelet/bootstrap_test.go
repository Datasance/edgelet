//go:build linux && full

package main

import (
	"errors"
	"strings"
	"testing"
	"time"
)

type fakeContainerdService struct {
	startErr   error
	stopCalled int
}

func (f *fakeContainerdService) Start() error { return f.startErr }
func (f *fakeContainerdService) Stop()        { f.stopCalled++ }

func TestStartEmbeddedContainerdWithRetryDeps_PreStartCleanupRunsBeforeFirstAttempt(t *testing.T) {
	cleanupCalls := 0
	attempts := 0

	deps := bootstrapDeps{
		ensureDependencies: func() error { return nil },
		newService: func() containerdStarter {
			attempts++
			return &fakeContainerdService{}
		},
		cleanupRuntime: func() error {
			cleanupCalls++
			return nil
		},
		sleep: func(time.Duration) {},
	}

	svc, err := startEmbeddedContainerdWithRetryDeps(deps)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if svc == nil {
		t.Fatal("expected non-nil service")
	}
	if attempts != 1 {
		t.Fatalf("expected single attempt success, got attempts=%d", attempts)
	}
	if cleanupCalls != 1 {
		t.Fatalf("expected exactly one pre-start cleanup call, got %d", cleanupCalls)
	}
}

func TestStartEmbeddedContainerdWithRetryDeps_SucceedsAfterRetry(t *testing.T) {
	attempt := 0
	cleanupCalls := 0

	deps := bootstrapDeps{
		ensureDependencies: func() error { return nil },
		newService: func() containerdStarter {
			attempt++
			if attempt < 2 {
				return &fakeContainerdService{startErr: errors.New("transient startup failure")}
			}
			return &fakeContainerdService{}
		},
		cleanupRuntime: func() error {
			cleanupCalls++
			return nil
		},
		sleep: func(time.Duration) {},
	}

	svc, err := startEmbeddedContainerdWithRetryDeps(deps)
	if err != nil {
		t.Fatalf("expected retry success, got error: %v", err)
	}
	if svc == nil {
		t.Fatal("expected non-nil service")
	}
	if attempt != 2 {
		t.Fatalf("expected 2 attempts, got %d", attempt)
	}
	if cleanupCalls != 2 {
		t.Fatalf("expected cleanup to run once before attempt and once between retries, got %d", cleanupCalls)
	}
}

func TestStartEmbeddedContainerdWithRetryDeps_FailsAfterMaxAttempts(t *testing.T) {
	attempt := 0
	cleanupCalls := 0

	deps := bootstrapDeps{
		ensureDependencies: func() error { return nil },
		newService: func() containerdStarter {
			attempt++
			return &fakeContainerdService{startErr: errors.New("persistent failure")}
		},
		cleanupRuntime: func() error {
			cleanupCalls++
			return nil
		},
		sleep: func(time.Duration) {},
	}

	svc, err := startEmbeddedContainerdWithRetryDeps(deps)
	if err == nil {
		t.Fatal("expected error after max attempts, got nil")
	}
	if svc != nil {
		t.Fatalf("expected nil service on failure, got %#v", svc)
	}
	if !strings.Contains(err.Error(), "did not become ready") {
		t.Fatalf("expected max-attempt error, got: %v", err)
	}
	if attempt != containerdBootstrapMaxAttempts {
		t.Fatalf("expected %d attempts, got %d", containerdBootstrapMaxAttempts, attempt)
	}
	expectedCleanupCalls := containerdBootstrapMaxAttempts // one pre-start + between retries
	if cleanupCalls != expectedCleanupCalls {
		t.Fatalf("expected cleanup calls=%d, got %d", expectedCleanupCalls, cleanupCalls)
	}
}
