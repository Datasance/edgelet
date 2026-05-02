//go:build linux

package iofogcontainerd

import (
	"github.com/eclipse-iofog/agent/internal/embedded"
)

// spinShimAvailable returns true if the containerd-shim-spin binary was embedded
// at compile time. On riscv64 the binary is nil because the spinframework project
// does not publish a riscv64 release.
func spinShimAvailable() bool {
	return len(embedded.ContainerdShimSpinBinary) > 0
}
