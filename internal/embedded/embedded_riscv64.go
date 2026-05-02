//go:build linux && riscv64

// Package embedded provides the binary assets for riscv64.
// containerd-shim-spin is excluded because the spinframework project does not
// publish riscv64 binaries. All other assets (runc, CNI, containerd-shim-runc-v2)
// are present and work normally.
package embedded

import _ "embed"

//go:embed bin/containerd/bin/containerd-shim-runc-v2
var ContainerdShimRuncBinary []byte

// ContainerdShimSpinBinary is nil on riscv64; the spin runtime will not be
// registered in the containerd config on this architecture.
var ContainerdShimSpinBinary []byte

//go:embed bin/runc
var RuncBinary []byte

//go:embed bin/cni/bridge
var CNIPluginBridge []byte

//go:embed bin/cni/host-local
var CNIPluginHostLocal []byte

//go:embed bin/cni/portmap
var CNIPluginPortmap []byte

//go:embed bin/cni/loopback
var CNIPluginLoopback []byte

// PauseImageTarGz is the portainer/pause:latest OCI image (gzip-compressed tar).
// Used as the CRI pod sandbox image. portainer/pause supports riscv64.
//
//go:embed bin/images/pause.tar.gz
var PauseImageTarGz []byte
