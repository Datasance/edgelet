//go:build linux && riscv64

// Package embedded provides the binary assets for riscv64.
package embedded

import _ "embed"

//go:embed bin/containerd/bin/containerd-shim-runc-v2
var ContainerdShimRuncBinary []byte

//go:embed bin/crun
var CrunBinary []byte

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
