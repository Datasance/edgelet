//go:build windows

package supervisor

import (
	"os"
	"syscall"
)

var signalSelfForSupervisor = func(sig syscall.Signal) error {
	proc, err := os.FindProcess(os.Getpid())
	if err != nil {
		return err
	}
	return proc.Signal(sig)
}
