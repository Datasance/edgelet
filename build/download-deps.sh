#!/bin/bash
# build/download-deps.sh
#
# Downloads and stages all binary dependencies that are embedded into iofog-agentd
# via Go's //go:embed directive. Must be run before `go build ./cmd/iofog-agentd`.
#
# Usage:
#   ./build/download-deps.sh [--os=linux] [--arch=amd64]
#
# Outputs to: internal/embedded/bin/
#   containerd/bin/containerd-shim-runc-v2
#   containerd-shim-spin            (skipped on riscv64)
#   crun
#   cni/bridge
#   cni/host-local
#   cni/portmap
#   cni/loopback
#   images/pause.tar.gz

set -euo pipefail

OS="linux"
ARCH="amd64"

# Dependency versions
CONTAINERD_VERSION="2.1.5"
CRUN_VERSION="1.27.1"
CNI_VERSION="v1.9.0"
SPIN_SHIM_VERSION="v0.24.0"

while [[ "$#" -gt 0 ]]; do
    case $1 in
        --os=*)  OS="${1#*=}";   shift ;;
        --os)    OS="$2";        shift 2 ;;
        --arch=*)ARCH="${1#*=}"; shift ;;
        --arch)  ARCH="$2";      shift 2 ;;
        *) echo "Unknown parameter: $1"; exit 1 ;;
    esac
done

echo "Downloading deps for OS=${OS}, ARCH=${ARCH}"

EMBED_DIR="internal/embedded/bin"
mkdir -p "${EMBED_DIR}/containerd/bin"
mkdir -p "${EMBED_DIR}/cni"
mkdir -p "${EMBED_DIR}/images"

# ─── containerd-shim-runc-v2 ─────────────────────────────────────────────────
if [ "${ARCH}" = "arm" ]; then
    # ARM32: containerd does not publish a standalone shim binary for armhf in
    # the standard containerd release; cross-compile via Docker.
    echo "Building containerd-shim-runc-v2 for ${OS}-${ARCH} via Docker cross-compile..."
    if ! command -v docker &>/dev/null; then
        echo "ERROR: Docker is required for ARM32 containerd build." >&2
        exit 1
    fi
    docker build \
        -f build/containerd.Dockerfile \
        --build-arg CONTAINERD_VERSION="v${CONTAINERD_VERSION}" \
        -t iofog-containerd-arm32-cross .
    CID=$(docker create iofog-containerd-arm32-cross)
    docker cp "${CID}:/containerd-v${CONTAINERD_VERSION}-${OS}-arm32.tar.gz" \
        "${EMBED_DIR}/containerd.tar.gz"
    docker rm "${CID}"
else
    echo "Downloading containerd ${CONTAINERD_VERSION} for ${OS}-${ARCH}..."
    curl -fsSL -o "${EMBED_DIR}/containerd.tar.gz" \
        "https://github.com/containerd/containerd/releases/download/v${CONTAINERD_VERSION}/containerd-${CONTAINERD_VERSION}-${OS}-${ARCH}.tar.gz"
fi

if ! tar -tf "${EMBED_DIR}/containerd.tar.gz" &>/dev/null; then
    echo "ERROR: Downloaded containerd archive is invalid." >&2
    exit 1
fi
tar -xzf "${EMBED_DIR}/containerd.tar.gz" -C "${EMBED_DIR}/containerd"
rm "${EMBED_DIR}/containerd.tar.gz"
echo "containerd-shim-runc-v2 extracted."

# ─── crun ────────────────────────────────────────────────────────────────────
CRUN_ARCH="${ARCH}"
if [ "${ARCH}" = "arm" ]; then
    CRUN_ARCH="arm"
elif [ "${ARCH}" = "riscv64" ]; then
    CRUN_ARCH="riscv64"
fi
CRUN_ASSET="crun-${CRUN_VERSION}-linux-${CRUN_ARCH}"
echo "Downloading crun ${CRUN_VERSION} for ${CRUN_ARCH}..."
curl -fsSL -o "${EMBED_DIR}/crun" \
    "https://github.com/containers/crun/releases/download/${CRUN_VERSION}/${CRUN_ASSET}"
chmod +x "${EMBED_DIR}/crun"
echo "crun downloaded."

# ─── CNI plugins ─────────────────────────────────────────────────────────────
echo "Downloading CNI plugins ${CNI_VERSION} for ${OS}-${ARCH}..."
curl -fsSL -o "${EMBED_DIR}/cni/cni-plugins.tgz" \
    "https://github.com/containernetworking/plugins/releases/download/${CNI_VERSION}/cni-plugins-${OS}-${ARCH}-${CNI_VERSION}.tgz"
if ! tar -tf "${EMBED_DIR}/cni/cni-plugins.tgz" &>/dev/null; then
    echo "ERROR: Downloaded CNI plugins archive is invalid." >&2
    exit 1
fi
tar -xzf "${EMBED_DIR}/cni/cni-plugins.tgz" -C "${EMBED_DIR}/cni"
rm "${EMBED_DIR}/cni/cni-plugins.tgz"
echo "CNI plugins extracted."

# ─── containerd-shim-spin (Wasm/WASI shim) ───────────────────────────────────
# containerd-shim-spin is not available for riscv64; skip it.
# The embedded_riscv64.go build-tag file will exclude it from the binary on that arch.
if [ "${ARCH}" = "riscv64" ]; then
    echo "Skipping containerd-shim-spin for riscv64 (not supported upstream)."
else
    # Map arch names to spinframework release asset names
    SPIN_ARCH="${ARCH}"
    case "${ARCH}" in
        amd64)   SPIN_ARCH="x86_64" ;;
        arm64)   SPIN_ARCH="aarch64" ;;
        arm)     SPIN_ARCH="armv7" ;;
    esac
    SPIN_ASSET="containerd-shim-spin-v2-linux-${SPIN_ARCH}.tar.gz"
    echo "Downloading containerd-shim-spin ${SPIN_SHIM_VERSION} for ${OS}-${ARCH} (${SPIN_ARCH})..."
    curl -fsSL -o "${EMBED_DIR}/spin-shim.tar.gz" \
        "https://github.com/spinframework/containerd-shim-spin/releases/download/${SPIN_SHIM_VERSION}/${SPIN_ASSET}"
    if ! tar -tf "${EMBED_DIR}/spin-shim.tar.gz" &>/dev/null; then
        echo "ERROR: Downloaded spin shim archive is invalid." >&2
        exit 1
    fi
    # The archive contains the binary named containerd-shim-spin-v2; rename to containerd-shim-spin
    tar -xzf "${EMBED_DIR}/spin-shim.tar.gz" -C "${EMBED_DIR}"
    # Locate and normalise the extracted binary name
    SPIN_BIN=$(find "${EMBED_DIR}" -maxdepth 1 -name 'containerd-shim-spin*' -type f | head -1)
    if [ -z "${SPIN_BIN}" ]; then
        echo "ERROR: Could not find containerd-shim-spin binary after extraction." >&2
        exit 1
    fi
    mv "${SPIN_BIN}" "${EMBED_DIR}/containerd-shim-spin"
    chmod +x "${EMBED_DIR}/containerd-shim-spin"
    rm "${EMBED_DIR}/spin-shim.tar.gz"
    echo "containerd-shim-spin downloaded."
fi

# ─── portainer/pause (sandbox image for CRI podsandbox) ───────────────────────
# Lightweight, multi-arch including riscv64. Required for CRI pod sandboxes.
echo "Checking for crane..."
if ! command -v crane &>/dev/null; then
    echo "Installing crane (go-containerregistry)..."
    CRANE_VERSION=$(curl -s "https://api.github.com/repos/google/go-containerregistry/releases/latest" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')
    CRANE_OS=$(uname -s)
    CRANE_ARCH_HOST=$(uname -m)
    case "${CRANE_ARCH_HOST}" in
        x86_64) CRANE_ARCH_HOST="x86_64" ;;
        aarch64|arm64) CRANE_ARCH_HOST="arm64" ;;
        *) CRANE_ARCH_HOST="x86_64" ;;
    esac
    CRANE_ASSET="go-containerregistry_${CRANE_OS}_${CRANE_ARCH_HOST}.tar.gz"
    curl -fsSL "https://github.com/google/go-containerregistry/releases/download/${CRANE_VERSION}/${CRANE_ASSET}" -o /tmp/crane.tar.gz
    tar -xzf /tmp/crane.tar.gz -C /tmp crane
    CRANE="/tmp/crane"
    rm -f /tmp/crane.tar.gz
else
    CRANE="crane"
fi

# Map ARCH to crane platform (linux/ARCH)
CRANE_ARCH="${ARCH}"
case "${ARCH}" in
    arm) CRANE_ARCH="arm/v7" ;;
esac

PAUSE_IMAGE="portainer/pause:latest"
echo "Downloading pause image ${PAUSE_IMAGE} for ${OS}-${CRANE_ARCH}..."
if ! "${CRANE}" pull --platform "${OS}/${CRANE_ARCH}" "${PAUSE_IMAGE}" "${EMBED_DIR}/images/pause.tar"; then
    echo "ERROR: Failed to pull pause image." >&2
    exit 1
fi
gzip -f "${EMBED_DIR}/images/pause.tar"
echo "Pause image downloaded."

echo ""
echo "All dependencies downloaded successfully for ${OS}-${ARCH}."
echo ""
echo "Staged assets:"
echo "  ${EMBED_DIR}/containerd/bin/containerd-shim-runc-v2"
echo "  ${EMBED_DIR}/crun"
echo "  ${EMBED_DIR}/cni/{bridge,host-local,portmap,loopback}"
echo "  ${EMBED_DIR}/images/pause.tar.gz"
[ "${ARCH}" != "riscv64" ] && echo "  ${EMBED_DIR}/containerd-shim-spin"
