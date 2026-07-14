//go:build linux && cgo

package main

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/eclipse-iofog/edgelet/pkg/containerd"
)

type fakeContainerdService struct {
	startErr   error
	stopCalled int
}

func (f *fakeContainerdService) Start() error { return f.startErr }
func (f *fakeContainerdService) Stop()        { f.stopCalled++ }

func TestStartEmbeddedContainerdWithRetryDeps_PrepareRunsBeforeEachAttempt(t *testing.T) {
	prepareCalls := 0
	artifactCleanupCalls := 0
	attempts := 0

	deps := bootstrapDeps{
		ensureDependencies: func() error { return nil },
		newService: func() containerdStarter {
			attempts++
			return &fakeContainerdService{}
		},
		cleanupRuntime: func() error {
			artifactCleanupCalls++
			return nil
		},
		prepareBootstrap: func() {
			prepareCalls++
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
	if prepareCalls != 1 {
		t.Fatalf("expected exactly one bootstrap prepare call, got %d", prepareCalls)
	}
	if artifactCleanupCalls != 0 {
		t.Fatalf("expected no artifact cleanup on first-attempt success, got %d", artifactCleanupCalls)
	}
}

func TestStartEmbeddedContainerdWithRetryDeps_SucceedsAfterRetry(t *testing.T) {
	attempt := 0
	prepareCalls := 0
	artifactCleanupCalls := 0

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
			artifactCleanupCalls++
			return nil
		},
		prepareBootstrap: func() {
			prepareCalls++
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
	if prepareCalls != 2 {
		t.Fatalf("expected bootstrap prepare before each attempt, got %d", prepareCalls)
	}
	if artifactCleanupCalls != 1 {
		t.Fatalf("expected artifact cleanup once between retries, got %d", artifactCleanupCalls)
	}
}

func TestStartEmbeddedContainerdWithRetryDeps_FailsAfterMaxAttempts(t *testing.T) {
	attempt := 0
	prepareCalls := 0
	artifactCleanupCalls := 0

	deps := bootstrapDeps{
		ensureDependencies: func() error { return nil },
		newService: func() containerdStarter {
			attempt++
			return &fakeContainerdService{startErr: errors.New("persistent failure")}
		},
		cleanupRuntime: func() error {
			artifactCleanupCalls++
			return nil
		},
		prepareBootstrap: func() {
			prepareCalls++
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
	if strings.Contains(err.Error(), "%!w(<nil>)") {
		t.Fatalf("expected explicit error text, got: %v", err)
	}
	if attempt != containerdBootstrapMaxAttempts {
		t.Fatalf("expected %d attempts, got %d", containerdBootstrapMaxAttempts, attempt)
	}
	if prepareCalls != containerdBootstrapMaxAttempts {
		t.Fatalf("expected prepare calls=%d, got %d", containerdBootstrapMaxAttempts, prepareCalls)
	}
	expectedArtifactCleanupCalls := containerdBootstrapMaxAttempts - 1
	if artifactCleanupCalls != expectedArtifactCleanupCalls {
		t.Fatalf("expected artifact cleanup calls=%d, got %d", expectedArtifactCleanupCalls, artifactCleanupCalls)
	}
}

func TestStartEmbeddedContainerdWithRetryDeps_SkipsArtifactCleanupWhenShimsRemain(t *testing.T) {
	attempt := 0
	artifactCleanupCalls := 0
	reapCalls := 0

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
			artifactCleanupCalls++
			return nil
		},
		prepareBootstrap: func() {},
		sleep:            func(time.Duration) {},
	}

	prevReap := reapManagedShimsBeforeBootstrapCleanup
	reapManagedShimsBeforeBootstrapCleanup = func(_ string, _ time.Duration) error {
		reapCalls++
		return errors.New("managed runtime processes still running after reap attempts: shims=[9999]")
	}
	defer func() { reapManagedShimsBeforeBootstrapCleanup = prevReap }()

	svc, err := startEmbeddedContainerdWithRetryDeps(deps)
	if err != nil {
		t.Fatalf("expected retry success, got error: %v", err)
	}
	if svc == nil {
		t.Fatal("expected non-nil service")
	}
	if reapCalls != 1 {
		t.Fatalf("expected one shim reap before retry cleanup, got %d", reapCalls)
	}
	if artifactCleanupCalls != 0 {
		t.Fatalf("expected artifact cleanup skipped while shims remain, got %d", artifactCleanupCalls)
	}
}

func TestWrapBootstrapContainerdStartErr_NilSafe(t *testing.T) {
	err := wrapBootstrapContainerdStartErr(nil)
	if err == nil {
		t.Fatal("expected error for nil input")
	}
	if strings.Contains(err.Error(), "%!w(<nil>)") {
		t.Fatalf("unexpected nil wrap artifact: %v", err)
	}
}

func TestWrapBootstrapContainerdStartErr_ClassifiesSpawnFailure(t *testing.T) {
	startErr := fmt.Errorf("%w: missing fat runtime", containerd.ErrContainerdSpawnFailure)
	err := wrapBootstrapContainerdStartErr(startErr)
	if err == nil {
		t.Fatal("expected wrapped error")
	}
	if !strings.Contains(err.Error(), "child spawn failed") {
		t.Fatalf("expected spawn classification, got: %v", err)
	}
	if !errors.Is(err, containerd.ErrContainerdSpawnFailure) {
		t.Fatalf("expected spawn error in chain, got: %v", err)
	}
}
