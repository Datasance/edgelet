//go:build linux && cgo

package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/eclipse-iofog/edgelet/internal/buildmeta"
	"github.com/eclipse-iofog/edgelet/internal/cgroups"
	"github.com/eclipse-iofog/edgelet/internal/config"
	"github.com/eclipse-iofog/edgelet/internal/constants"
	"github.com/eclipse-iofog/edgelet/internal/utils"
	"github.com/eclipse-iofog/edgelet/internal/utils/logging"
	"github.com/eclipse-iofog/edgelet/pkg/data"
)

func runRuntimeBootstrap() {
	if !buildmeta.HasEmbeddedEngine() {
		_, _ = fmt.Fprint(os.Stderr, "runtime-bootstrap requires embedded engine build\n")
		exitDaemon(1)
	}

	if err := config.LoadConfig(utils.ConfigYAMLPath); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		exitDaemon(1)
	}
	cfg := config.GetInstance()
	if err := config.ValidateConfig(cfg); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Configuration validation failed: %v\n", err)
		exitDaemon(1)
	}
	if cfg.ContainerEngine != constants.EngineEdgelet {
		_, _ = fmt.Fprintf(os.Stderr, "runtime-bootstrap requires containerEngine=edgelet (got %q)\n", cfg.ContainerEngine)
		exitDaemon(1)
	}

	setupEnvironment()
	startLoggingService(logging.BasenameDataPlane)

	if _, err := cgroups.Bootstrap(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Failed to bootstrap cgroups for embedded engine: %v\n", err)
		exitDaemon(1)
	}

	svc, err := startEmbeddedContainerdWithRetry()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Failed to start embedded containerd: %v\n", err)
		exitDaemon(1)
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)

	logging.LogInfo("RUNTIME_BOOTSTRAP", "Embedded containerd data plane running")

	for {
		sig := <-sigChan
		switch sig {
		case syscall.SIGHUP:
			reloadRuntimeBootstrapConfig()
			continue
		case syscall.SIGTERM, syscall.SIGINT:
			logging.LogInfo("RUNTIME_BOOTSTRAP", "Stopping embedded containerd data plane")

			if err := data.EnsureExtracted(); err != nil {
				logging.LogWarn("RUNTIME_BOOTSTRAP", fmt.Sprintf("Runtime bundle refresh before drain skipped: %v", err))
			}

			drainSec := cfg.ShutdownDrainTimeout()
			logging.LogInfo("RUNTIME_BOOTSTRAP", fmt.Sprintf("Data-plane stop grace: %ds", drainSec))
			svc.Stop()
			logging.LogInfo("RUNTIME_BOOTSTRAP", "Embedded containerd data plane stopped")
			return
		}
	}
}

func reloadRuntimeBootstrapConfig() {
	logging.LogInfo("RUNTIME_BOOTSTRAP", "Reloading configuration due to SIGHUP")
	if err := config.LoadConfig(utils.ConfigYAMLPath); err != nil {
		logging.LogError("RUNTIME_BOOTSTRAP", "Failed to reload configuration", err)
		return
	}
	cfg := config.GetInstance()
	if err := config.ValidateConfig(cfg); err != nil {
		logging.LogError("RUNTIME_BOOTSTRAP", "Configuration validation failed after reload", err)
		return
	}
	budgetMB := logging.DaemonLogBudgetMB(cfg.LogDiskLimit, logging.SeriesDataPlane, true)
	if err := logging.InstanceConfigUpdated(cfg.LogDiskDirectory, budgetMB, cfg.LogFileCount, cfg.LogLevel, logging.BasenameDataPlane); err != nil {
		logging.LogError("RUNTIME_BOOTSTRAP", "Failed to update logger configuration", err)
		return
	}
	logging.LogInfo("RUNTIME_BOOTSTRAP", "Configuration reloaded successfully")
}
