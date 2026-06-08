//go:build !linux && unix

package main

import (
	"github.com/eclipse-iofog/edgelet/internal/supervisor"
	"github.com/eclipse-iofog/edgelet/internal/utils/logging"
)

func onConfigFileChanged(_ *supervisor.Supervisor) {
	logging.LogInfo("Daemon", "Sending SIGHUP to trigger config reload...")
	if err := requestConfigReload(); err != nil {
		logging.LogError("Daemon", "Failed to send SIGHUP for config reload", err)
	}
}
