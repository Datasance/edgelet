//go:build unix

package supervisor

import (
	"os"
	"syscall"
)

var signalSelfForSupervisor = func(sig syscall.Signal) error {
	return syscall.Kill(os.Getpid(), sig)
}
