package supervisor

import (
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eclipse-iofog/edgelet/internal/constants"
	"github.com/eclipse-iofog/edgelet/pkg/engine"
)

type retryStubEngine struct {
	engine.ContainerEngine
	initCalls atomic.Int32
	failUntil int32
}

func (e *retryStubEngine) Init(_ engine.EngineConfig) error {
	call := e.initCalls.Add(1)
	if call <= e.failUntil {
		return fmt.Errorf("socket unavailable (attempt %d)", call)
	}
	return nil
}

func TestEngineInitTotalWaitBudget(t *testing.T) {
	const want = 4 * time.Minute
	if got := engineInitTotalWaitBudget(); got != want {
		t.Fatalf("engineInitTotalWaitBudget() = %v, want %v", got, want)
	}
}

func TestInitExternalEngineWithRetry_SucceedsAfterTransientFailures(t *testing.T) {
	stub := &retryStubEngine{failUntil: 2}
	origNew := supervisorNewContainerEngine
	origWait := engineInitRetryWait
	t.Cleanup(func() {
		supervisorNewContainerEngine = origNew
		engineInitRetryWait = origWait
	})
	supervisorNewContainerEngine = func(engineType string, _ engine.EngineConfig) (engine.ContainerEngine, error) {
		if engineType != constants.EngineDocker {
			return nil, fmt.Errorf("unexpected engine type %q", engineType)
		}
		return stub, nil
	}
	engineInitRetryWait = func(time.Duration) {}

	s := NewSupervisor()
	eng, err := s.initExternalEngineWithRetry(constants.EngineDocker, engine.EngineConfig{
		SocketURL: "unix:///var/run/docker.sock",
	})
	if err != nil {
		t.Fatalf("initExternalEngineWithRetry() error = %v", err)
	}
	if eng == nil {
		t.Fatal("expected engine, got nil")
	}
	if got := stub.initCalls.Load(); got != 3 {
		t.Fatalf("Init calls = %d, want 3", got)
	}
}

func TestInitExternalEngineWithRetry_ExhaustedReturnsClearError(t *testing.T) {
	stub := &retryStubEngine{failUntil: engineInitMaxRetries}
	origNew := supervisorNewContainerEngine
	origWait := engineInitRetryWait
	t.Cleanup(func() {
		supervisorNewContainerEngine = origNew
		engineInitRetryWait = origWait
	})
	supervisorNewContainerEngine = func(_ string, _ engine.EngineConfig) (engine.ContainerEngine, error) {
		return stub, nil
	}
	engineInitRetryWait = func(time.Duration) {}

	s := NewSupervisor()
	_, err := s.initExternalEngineWithRetry(constants.EnginePodman, engine.EngineConfig{
		SocketURL: "unix:///run/podman/podman.sock",
	})
	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}
	msg := err.Error()
	if strings.Contains(strings.ToLower(msg), "edgelet") {
		t.Fatalf("error must not suggest edgelet fallback: %q", msg)
	}
	if !strings.Contains(msg, fmt.Sprintf("%d init attempts", engineInitMaxRetries)) {
		t.Fatalf("error should mention attempt count: %q", msg)
	}
	if !strings.Contains(msg, engineInitTotalWaitBudget().String()) {
		t.Fatalf("error should mention retry wait budget: %q", msg)
	}
	if !strings.Contains(msg, "socket unavailable") {
		t.Fatalf("error should wrap last init failure: %q", msg)
	}
	if got := stub.initCalls.Load(); got != engineInitMaxRetries {
		t.Fatalf("Init calls = %d, want %d", got, engineInitMaxRetries)
	}
}
