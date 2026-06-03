//go:build linux && cgo

package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/datasance/edgelet/internal/buildmeta"
	"github.com/datasance/edgelet/internal/cgroups"
	"github.com/datasance/edgelet/internal/config"
	"github.com/datasance/edgelet/internal/constants"
	"github.com/datasance/edgelet/internal/utils"
	"github.com/datasance/edgelet/internal/utils/logging"
	"github.com/datasance/edgelet/pkg/data"
)

func runRuntimeBootstrap() {
	if !buildmeta.HasEmbeddedEngine() {
		fmt.Fprintf(os.Stderr, "runtime-bootstrap requires embedded engine build\n")
		os.Exit(1)
	}

	if err := config.LoadConfig(utils.ConfigYAMLPath); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		os.Exit(1)
	}
	cfg := config.GetInstance()
	if err := config.ValidateConfig(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Configuration validation failed: %v\n", err)
		os.Exit(1)
	}
	if cfg.ContainerEngine != constants.EngineEdgelet {
		fmt.Fprintf(os.Stderr, "runtime-bootstrap requires containerEngine=edgelet (got %q)\n", cfg.ContainerEngine)
		os.Exit(1)
	}

	setupEnvironment()
	startLoggingService()

	if _, err := cgroups.Bootstrap(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to bootstrap cgroups for embedded engine: %v\n", err)
		os.Exit(1)
	}

	svc, err := startEmbeddedContainerdWithRetry()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start embedded containerd: %v\n", err)
		os.Exit(1)
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)

	logging.LogInfo("RUNTIME_BOOTSTRAP", "Embedded containerd data plane running")

	<-sigChan
	logging.LogInfo("RUNTIME_BOOTSTRAP", "Stopping embedded containerd data plane")

	if err := data.EnsureExtracted(); err != nil {
		logging.LogWarn("RUNTIME_BOOTSTRAP", fmt.Sprintf("Runtime bundle refresh before drain skipped: %v", err))
	}

	drainSec := cfg.ShutdownDrainTimeout()
	logging.LogInfo("RUNTIME_BOOTSTRAP", fmt.Sprintf("Data-plane stop grace: %ds", drainSec))
	svc.Stop()
	logging.LogInfo("RUNTIME_BOOTSTRAP", "Embedded containerd data plane stopped")
}
