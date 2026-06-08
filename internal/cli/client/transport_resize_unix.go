//go:build unix

package client

import (
	"os"
	"os/signal"
	"syscall"
)

// watchExecResize listens for SIGWINCH and invokes sendResize on terminal size changes.
// The returned stop function blocks until the watcher exits.
func watchExecResize(sendResize func()) func() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGWINCH)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range sigCh {
			sendResize()
		}
	}()
	return func() {
		signal.Stop(sigCh)
		close(sigCh)
		<-done
	}
}
