//go:build windows

package client

// watchExecResize is a no-op on Windows (no SIGWINCH); initial resize is sent by the caller.
func watchExecResize(_ func()) func() {
	return func() {}
}
