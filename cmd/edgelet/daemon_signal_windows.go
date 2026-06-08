//go:build windows

package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/eclipse-iofog/edgelet/internal/supervisor"
)

func registerDaemonSignals(sigChan chan os.Signal) {
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)
}

func onConfigFileChanged(sup *supervisor.Supervisor) {
	reloadAgentConfig(sup)
}

func isConfigReloadSignal(_ os.Signal) bool {
	return false
}
