package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/eclipse-iofog/agent/internal/buildmeta"
	"github.com/eclipse-iofog/agent/internal/config"
	"github.com/eclipse-iofog/agent/internal/constants"
	"github.com/eclipse-iofog/agent/internal/supervisor"
	"github.com/eclipse-iofog/agent/internal/utils"
	"github.com/eclipse-iofog/agent/internal/utils/logging"
	iofogcontainerd "github.com/eclipse-iofog/agent/pkg/containerd"
)

var (
	version   = "dev"
	buildTime = "unknown"
	gitCommit = "unknown"
)

func main() {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "iofog-agentd panic: %v\n", r)
			os.Exit(1)
		}
	}()

	// Internal child-process mode: run containerd and exit.
	if handled, err := iofogcontainerd.MaybeRunChildProcess(os.Args); handled {
		if err != nil {
			fmt.Fprintf(os.Stderr, "Embedded containerd child failed: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	// Version (no config required)
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "version", "--version", "-v":
			printDaemonVersion()
			os.Exit(0)
		}
	}

	// Load configuration
	if err := config.LoadConfig(utils.ConfigYAMLPath); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// Validate configuration on startup
	if err := config.ValidateConfig(config.GetInstance()); err != nil {
		fmt.Fprintf(os.Stderr, "Configuration validation failed: %v\n", err)
		os.Exit(1)
	}

	// Setup environment
	setupEnvironment()

	// Check if another instance is running
	if utils.IsAnotherInstanceRunning() {
		fmt.Println("ioFog Agent is already running.")
		os.Exit(0)
	}

	// Start logging service
	startLoggingService()

	// Write PID file
	if err := utils.WritePIDFile(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write PID file: %v\n", err)
		os.Exit(1)
	}
	defer utils.RemovePIDFile()

	cfg := config.GetInstance()

	// Bootstrap embedded containerd in main before Supervisor (full build + iofog engine).
	var prestarted *iofogcontainerd.Service
	if buildmeta.IsFull() && cfg.ContainerEngine == constants.EngineIofog {
		var err error
		prestarted, err = startEmbeddedContainerdWithRetry()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to start embedded containerd: %v\n", err)
			os.Exit(1)
		}
		logging.LogInfo("MAIN_DAEMON", "Embedded containerd started before Supervisor")
	}

	// Create and start supervisor
	sup := supervisor.NewSupervisor()
	if prestarted != nil {
		sup.SetPrestartedContainerd(prestarted)
	}
	if err := sup.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start supervisor: %v\n", err)
		os.Exit(1)
	}

	// Start configuration watcher (matching Java: dynamic configuration update)
	watcher := config.GetWatcher()
	if err := watcher.Watch(context.Background(), utils.ConfigYAMLPath); err != nil {
		logging.LogError("Daemon", "Failed to start config watcher", err)
	} else {
		watcher.OnChange(func() {
			if config.IsReloadSuppressedForDeprovision() {
				logging.LogDebug("Daemon", "Skipping SIGHUP for deprovision config save")
				return
			}
			logging.LogInfo("Daemon", "Configuration file changed, sending SIGHUP to trigger reload...")
			// Send SIGHUP to self to trigger reload through the main signal handler
			// This ensures serialization and debouncing
			if err := syscall.Kill(os.Getpid(), syscall.SIGHUP); err != nil {
				logging.LogError("Daemon", "Failed to send SIGHUP for config reload", err)
			}
		})
	}

	// Setup signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)

	// Last reload time for debouncing
	var lastReloadTime time.Time
	const reloadDebounceInterval = 2 * time.Second

	// Wait for signals
	for {
		sig := <-sigChan
		logging.LogInfo("Daemon", fmt.Sprintf("Received signal: %v", sig))

		if sig == syscall.SIGHUP {
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

		// Graceful shutdown with configurable grace period
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
				os.Exit(1)
			}
			logging.LogInfo("Daemon", "Shutdown complete")
		case <-time.After(gracePeriod):
			logging.LogWarn("Daemon", fmt.Sprintf("Shutdown grace period (%v) exceeded, exiting", gracePeriod))
		}
		os.Exit(0)
	}
}

func setupEnvironment() {
	// 0755 is intentional: /var/run must be world-traversable for PID file access
	mkErr := os.MkdirAll(utils.VarRun, 0755) // #nosec G301
	if mkErr != nil {
		fmt.Fprintf(os.Stderr, "Failed to create var/run directory: %v\n", mkErr)
		os.Exit(1)
	}
}

func startLoggingService() {
	logo := "\n" +
		"  _        __                                     _   \n" +
		" (_)      / _|                                   | |  \n" +
		"  _  ___ | |_ ___   __ _    __ _  __ _  ___ _ __ | |_ \n" +
		" | |/ _ \\|  _/ _ \\ / _` |  / _` |/ _` |/ _ \\ '_ \\| __|\n" +
		" | | (_) | || (_) | (_| | | (_| | (_| |  __/ | | | |_ \n" +
		" |_|\\___/|_| \\___/ \\__, |  \\__,_|\\__, |\\___|_| |_|\\__|\n" +
		"                    __/ |         __/ |               \n" +
		"                   |___/         |___/                \n" +
		"                                                                                \n" +
		"  Datasance PoT ioFog Agent v" + version + " (build: " + buildTime + ", commit: " + gitCommit + ")\n" +
		"  Logging Service Started\n"

	cfg := config.GetInstance()
	logo += fmt.Sprintf("  Log Level: %s\n", cfg.LogLevel)
	logo += fmt.Sprintf("  Log Directory: %s\n", cfg.LogDiskDirectory)

	fmt.Print(logo)

	// Setup logger with configuration (matching Java: LoggingService.setupLogger())
	// Convert log disk limit from GiB to MB
	logDiskLimitMB := int(cfg.LogDiskLimit * 1024)
	if err := logging.SetupLogger(cfg.LogDiskDirectory, logDiskLimitMB, cfg.LogFileCount, cfg.LogLevel); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to setup logger: %v\n", err)
		os.Exit(1)
	}

	logging.LogInfo("MAIN_DAEMON", "Configuration loaded.")
}

func printDaemonVersion() {
	fmt.Printf("iofog-agentd %s (build: %s, commit: %s)\n", version, buildTime, gitCommit)
	fmt.Printf("  build flavor: %s\n", buildmeta.Flavor)
	fmt.Printf("  allowed containerEngine: %s\n", buildmeta.AllowedEnginesCSV())
}

// reloadAgentConfig handles the complete agent configuration reload process
func reloadAgentConfig(sup *supervisor.Supervisor) {
	logging.LogInfo("Daemon", "Reloading configuration...")
	config.SetLastReloadSuccessful(false)

	// Reload configuration from file
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

	// Update logger
	logDiskLimitMB := int(cfg.LogDiskLimit * 1024)
	if err := logging.InstanceConfigUpdated(cfg.LogDiskDirectory, logDiskLimitMB, cfg.LogFileCount, cfg.LogLevel); err != nil {
		logging.LogError("Daemon", "Failed to update logger configuration", err)
	}

	// Notify supervisor to update all modules
	if err := sup.ReloadConfig(); err != nil {
		logging.LogError("Daemon", "Failed to notify modules of config reload", err)
	}

	logging.LogInfo("Daemon", "Configuration reloaded successfully")
}
