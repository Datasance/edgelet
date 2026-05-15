package main

import (
	"fmt"
	"time"

	"github.com/eclipse-iofog/agent/internal/embedded"
	"github.com/eclipse-iofog/agent/internal/utils/logging"
	iofogcontainerd "github.com/eclipse-iofog/agent/pkg/containerd"
)

const (
	containerdBootstrapMaxAttempts = 3
	containerdBootstrapBaseBackoff = 2 * time.Second
	containerdBootstrapMaxBackoff  = 10 * time.Second
)

type containerdStarter interface {
	Start() error
	Stop()
}

type bootstrapDeps struct {
	ensureDependencies func() error
	newService         func() containerdStarter
	cleanupRuntime     func() error
	sleep              func(time.Duration)
}

func startEmbeddedContainerdWithRetry() (*iofogcontainerd.Service, error) {
	deps := bootstrapDeps{
		ensureDependencies: embedded.EnsureEmbeddedDependencies,
		newService: func() containerdStarter {
			return iofogcontainerd.NewService()
		},
		cleanupRuntime: iofogcontainerd.CleanupRuntimeArtifacts,
		sleep:          time.Sleep,
	}

	svc, err := startEmbeddedContainerdWithRetryDeps(deps)
	if err != nil {
		return nil, err
	}

	typed, ok := svc.(*iofogcontainerd.Service)
	if !ok {
		return nil, fmt.Errorf("unexpected embedded containerd service type %T", svc)
	}
	return typed, nil
}

func startEmbeddedContainerdWithRetryDeps(deps bootstrapDeps) (containerdStarter, error) {
	var lastErr error
	backoff := containerdBootstrapBaseBackoff

	// Pre-start cleanup before the first attempt helps clear stale runtime
	// artifacts after crashes/reboots instead of waiting for attempt-1 failure.
	if err := deps.cleanupRuntime(); err != nil {
		logging.LogWarn("MAIN_DAEMON", fmt.Sprintf("Embedded containerd pre-start runtime cleanup failed: %v", err))
	}

	for attempt := 1; attempt <= containerdBootstrapMaxAttempts; attempt++ {
		if err := deps.ensureDependencies(); err != nil {
			lastErr = fmt.Errorf("prepare embedded dependencies: %w", err)
		} else {
			svc := deps.newService()
			if err := svc.Start(); err == nil {
				return svc, nil
			} else {
				lastErr = fmt.Errorf("start embedded containerd: %w", err)
				svc.Stop()
			}
		}

		logging.LogWarn("MAIN_DAEMON", fmt.Sprintf(
			"Embedded containerd startup attempt %d/%d failed: %v",
			attempt, containerdBootstrapMaxAttempts, lastErr,
		))

		if attempt == containerdBootstrapMaxAttempts {
			break
		}

		if err := deps.cleanupRuntime(); err != nil {
			logging.LogWarn("MAIN_DAEMON", fmt.Sprintf("Embedded containerd runtime cleanup failed before retry: %v", err))
		}

		deps.sleep(backoff)
		if backoff < containerdBootstrapMaxBackoff {
			backoff *= 2
			if backoff > containerdBootstrapMaxBackoff {
				backoff = containerdBootstrapMaxBackoff
			}
		}
	}

	return nil, fmt.Errorf("embedded containerd did not become ready after %d attempts: %w", containerdBootstrapMaxAttempts, lastErr)
}
