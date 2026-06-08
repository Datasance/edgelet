//go:build !linux

package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/datasance/edgelet/internal/branding"
	"github.com/datasance/edgelet/internal/buildmeta"
	"github.com/datasance/edgelet/internal/config"
	"github.com/datasance/edgelet/internal/constants"
	"github.com/datasance/edgelet/internal/supervisor"
	"github.com/datasance/edgelet/internal/utils"
	"github.com/datasance/edgelet/internal/utils/logging"
	"github.com/datasance/edgelet/pkg/containerd"
)

// exitDaemon removes the PID file then exits. defer does not run on os.Exit.
func exitDaemon(code int) {
	_ = utils.RemovePIDFile()
	os.Exit(code) //nolint:revive // deep-exit: centralized daemon exit helper
}

func runDaemon() {
	if err := config.LoadConfig(utils.ConfigYAMLPath); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		exitDaemon(1)
	}

	if err := config.ValidateConfig(config.GetInstance()); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Configuration validation failed: %v\n", err)
		exitDaemon(1)
	}

	setupEnvironment()
	startLoggingService()

	if utils.IsAnotherInstanceRunning() {
		_, _ = fmt.Println("Edgelet is already running.")
		exitDaemon(0)
	}

	if err := utils.WritePIDFile(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Failed to write PID file: %v\n", err)
		exitDaemon(1)
	}
	defer func() { _ = utils.RemovePIDFile() }()

	cfg := config.GetInstance()

	var prestarted *containerd.Service
	if buildmeta.HasEmbeddedEngine() && cfg.ContainerEngine == constants.EngineEdgelet {
		var err error
		prestarted, err = startEmbeddedContainerdWithRetry()
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "Failed to start embedded containerd: %v\n", err)
			exitDaemon(1)
		}
		logging.LogInfo("MAIN_DAEMON", "Embedded containerd started before Supervisor")
	}

	sup := supervisor.NewSupervisor()
	if prestarted != nil {
		sup.SetPrestartedContainerd(prestarted)
	}
	if err := sup.Start(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Failed to start supervisor: %v\n", err)
		exitDaemon(1)
	}

	watcher := config.GetWatcher()
	if err := watcher.Watch(context.Background(), utils.ConfigYAMLPath); err != nil {
		logging.LogError("Daemon", "Failed to start config watcher", err)
	} else {
		watcher.OnChange(func() {
			if config.IsReloadSuppressedForDeprovision() {
				logging.LogDebug("Daemon", "Skipping config reload for deprovision config save")
				return
			}
			if config.IsReloadSuppressedForInProcessMutation() {
				logging.LogDebug("Daemon", "Skipping config reload for in-process config mutation")
				return
			}
			logging.LogInfo("Daemon", "Configuration file changed, triggering reload...")
			onConfigFileChanged(sup)
		})
	}

	sigChan := make(chan os.Signal, 1)
	registerDaemonSignals(sigChan)

	var lastReloadTime time.Time
	const reloadDebounceInterval = 2 * time.Second

	for {
		sig := <-sigChan
		logging.LogInfo("Daemon", fmt.Sprintf("Received signal: %v", sig))

		if isConfigReloadSignal(sig) {
			now := time.Now()
			if now.Sub(lastReloadTime) < reloadDebounceInterval {
				logging.LogInfo("Daemon", "Ignoring SIGHUP (debounce)")
				continue
			}
			lastReloadTime = now

			logging.LogInfo("Daemon", "Reloading configuration due to SIGHUP")
			reloadAgentConfig(sup)
			continue
		}

		logging.LogInfo("Daemon", "Shutting down...")
		cfg := config.GetInstance()
		gracePeriod := time.Duration(cfg.ShutdownGracePeriodSeconds) * time.Second
		if gracePeriod < time.Second {
			gracePeriod = 90 * time.Second
		}

		done := make(chan struct{})
		var stopErr error
		go func() {
			defer close(done)
			stopErr = sup.Stop()
		}()

		select {
		case <-done:
			if stopErr != nil {
				logging.LogError("Daemon", "Error during shutdown", stopErr)
				exitDaemon(1)
			}
			logging.LogInfo("Daemon", "Shutdown complete")
		case <-time.After(gracePeriod):
			logging.LogWarn("Daemon", fmt.Sprintf("Shutdown grace period (%v) exceeded, exiting", gracePeriod))
		}
		exitDaemon(0)
	}
}

func setupEnvironment() {
	mkErr := os.MkdirAll(utils.VarRun, 0755) // #nosec G301
	if mkErr != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Failed to create var/run directory: %v\n", mkErr)
		exitDaemon(1)
	}
}

func startLoggingService() {
	logo := "\n" + branding.EdgeletANSIShadow + "\n" +
		"  Edgelet v" + version + " (build: " + buildTime + ", commit: " + gitCommit + ")\n" +
		"  Logging Service Started\n"

	cfg := config.GetInstance()
	logo += fmt.Sprintf("  Log Level: %s\n", cfg.LogLevel)
	logo += fmt.Sprintf("  Log Directory: %s\n", cfg.LogDiskDirectory)
	_, _ = fmt.Print(logo)

	logDiskLimitMB := int(cfg.LogDiskLimit * 1024)
	if err := logging.SetupLogger(cfg.LogDiskDirectory, logDiskLimitMB, cfg.LogFileCount, cfg.LogLevel); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Failed to setup logger: %v\n", err)
		exitDaemon(1)
	}

	logging.LogInfo("MAIN_DAEMON", "Configuration loaded.")
}

func reloadAgentConfig(sup *supervisor.Supervisor) {
	logging.LogInfo("Daemon", "Reloading configuration...")
	config.SetLastReloadSuccessful(false)
	sup.BeginConfigReload()

	if err := config.LoadConfig(utils.ConfigYAMLPath); err != nil {
		logging.LogError("Daemon", "Failed to reload configuration", err)
		logging.LogWarn("Daemon", "Rejected configuration reload; keeping last-known-good runtime config")
		return
	}

	cfg := config.GetInstance()
	if err := config.ValidateConfig(cfg); err != nil {
		logging.LogError("Daemon", "Configuration validation failed after reload", err)
		logging.LogWarn("Daemon", "Rejected configuration reload; keeping last-known-good runtime config")
		return
	}
	config.SetLastReloadSuccessful(true)

	logDiskLimitMB := int(cfg.LogDiskLimit * 1024)
	if err := logging.InstanceConfigUpdated(cfg.LogDiskDirectory, logDiskLimitMB, cfg.LogFileCount, cfg.LogLevel); err != nil {
		logging.LogError("Daemon", "Failed to update logger configuration", err)
	}

	if err := sup.ReloadConfig(); err != nil {
		logging.LogError("Daemon", "Failed to notify modules of config reload", err)
	}

	logging.LogInfo("Daemon", "Configuration reloaded successfully")
}
