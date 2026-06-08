//go:build !linux && unix

package main

import (
	"os"
	"os/signal"
	"syscall"
)

func registerDaemonSignals(sigChan chan os.Signal) {
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
}

func requestConfigReload() error {
	return syscall.Kill(os.Getpid(), syscall.SIGHUP)
}

func isConfigReloadSignal(sig os.Signal) bool {
	return sig == syscall.SIGHUP
}
