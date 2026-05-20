//go:build (linux && amd64) || (linux && arm64) || (linux && arm)

// Package embedded provides the binary assets that are compiled directly into
// iofog-agentd when containerEngine=iofog is selected. All assets are staged
// into internal/embedded/bin/ by running `build/download-deps.sh` before build.
package embedded

import _ "embed"

// containerd-shim-runc-v2 is the OCI runtime shim binary for containerd.
//
//go:embed bin/containerd/bin/containerd-shim-runc-v2
var ContainerdShimRuncBinary []byte

// crun is the low-level OCI container runtime binary.
//
//go:embed bin/crun
var CrunBinary []byte

// CNI plugin binaries — bridge, host-local, portmap, loopback.
//
//go:embed bin/cni/bridge
var CNIPluginBridge []byte

//go:embed bin/cni/host-local
var CNIPluginHostLocal []byte

//go:embed bin/cni/portmap
var CNIPluginPortmap []byte

//go:embed bin/cni/loopback
var CNIPluginLoopback []byte

// PauseImageTarGz is the portainer/pause:latest OCI image (gzip-compressed tar).
// Used as the CRI pod sandbox image. Embedded for all amd64/arm64/arm builds.
//
//go:embed bin/images/pause.tar.gz
var PauseImageTarGz []byte
