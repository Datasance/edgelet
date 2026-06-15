//go:build windows

package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/eclipse-iofog/edgelet/internal/supervisor"
	"github.com/eclipse-iofog/edgelet/internal/utils/logging"
)

func registerDaemonSignals(sigChan chan os.Signal) {
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)
}

func onConfigFileChanged(sup *supervisor.Supervisor) {
	if err := sup.ReloadFromDisk(); err != nil {
		logging.LogError("Daemon", "Configuration reload failed", err)
	}
}

func isConfigReloadSignal(_ os.Signal) bool {
	return false
}
