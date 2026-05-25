//go:build linux && full

package main

import (
	"fmt"
	"time"

	"github.com/datasance/edgelet/internal/embedded"
	"github.com/datasance/edgelet/internal/utils/logging"
	edgeletcontainerdd "github.com/datasance/edgelet/pkg/containerd"
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

// ensureEmbeddedDeps extracts embedded runtime binaries before containerd start.
// Integrator: replace this hook with pkg/data.EnsureExtracted from Plan 2.
func ensureEmbeddedDeps() error {
	return embedded.EnsureEmbeddedDependencies()
}

func startEmbeddedContainerdWithRetry() (*edgeletcontainerdd.Service, error) {
	deps := bootstrapDeps{
		ensureDependencies: ensureEmbeddedDeps,
		newService: func() containerdStarter {
			return edgeletcontainerdd.NewService()
		},
		cleanupRuntime: edgeletcontainerdd.CleanupRuntimeArtifacts,
		sleep:          time.Sleep,
	}

	svc, err := startEmbeddedContainerdWithRetryDeps(deps)
	if err != nil {
		return nil, err
	}

	typed, ok := svc.(*edgeletcontainerdd.Service)
	if !ok {
		return nil, fmt.Errorf("unexpected embedded containerd service type %T", svc)
	}
	return typed, nil
}

func startEmbeddedContainerdWithRetryDeps(deps bootstrapDeps) (containerdStarter, error) {
	var lastErr error
	backoff := containerdBootstrapBaseBackoff

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
			}
			lastErr = fmt.Errorf("start embedded containerd: %w", err)
			svc.Stop()
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
