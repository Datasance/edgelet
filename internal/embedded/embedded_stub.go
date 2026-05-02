//go:build !(linux && amd64) && !(linux && arm64) && !(linux && arm) && !(linux && riscv64)

// Package embedded provides stubs for platforms where the iofog embedded engine
// is not supported (e.g. macOS, Windows).
package embedded

import "fmt"

// EnsureEmbeddedDependencies is a no-op stub on unsupported platforms.
func EnsureEmbeddedDependencies() error {
	return fmt.Errorf("iofog embedded engine is only supported on Linux (amd64, arm64, arm, riscv64)")
}

// Stub variable declarations so non-Linux code can reference the package without errors.
var (
	ContainerdShimRuncBinary []byte
	ContainerdShimSpinBinary []byte
	RuncBinary               []byte
	CNIPluginBridge          []byte
	CNIPluginHostLocal       []byte
	CNIPluginPortmap         []byte
	CNIPluginLoopback        []byte
)
