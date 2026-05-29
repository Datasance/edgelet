#!/usr/bin/env bash
# test/embedded/build.sh
#
# Runs the Plan 6 two-layer embed pipeline and cross-compiles the unified linux
# thin edgelet binary for the Linux target that will run inside the Lima VM.
#
# Output:
#   build/edgelet-linux-<arch>
#
# Usage:
#   ./test/embedded/build.sh [--arch=arm64|amd64]

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
source "${SCRIPT_DIR}/lib/log.sh"

###############################################################################
# Detect target architecture
###############################################################################
HOST_ARCH="$(native_arch)"
case "${HOST_ARCH}" in
    arm64|aarch64) DEFAULT_ARCH="arm64" ;;
    x86_64)        DEFAULT_ARCH="amd64" ;;
    *)             DEFAULT_ARCH="amd64" ;;
esac

TARGET_ARCH="${DEFAULT_ARCH}"
for arg in "$@"; do
    case "${arg}" in
        --arch=*) TARGET_ARCH="${arg#*=}" ;;
    esac
done

log_step "Building edgelet for linux/${TARGET_ARCH}"
log_info "Repository root: ${REPO_ROOT}"
log_info "Target arch: ${TARGET_ARCH}"

cd "${REPO_ROOT}"
chmod +x scripts/download scripts/download-root scripts/stage-root-aux \
    scripts/build-crun-static scripts/build-embedded scripts/package-data \
    scripts/build-edgelet 2>/dev/null || true

run_embed_pipeline() {
    ARCH="${TARGET_ARCH}" ./scripts/download
    ARCH="${TARGET_ARCH}" ./scripts/build-embedded
    ARCH="${TARGET_ARCH}" ./scripts/build-edgelet fat
    ARCH="${TARGET_ARCH}" ./scripts/package-data
}

###############################################################################
# Step 1: Plan 6 embed pipeline (download → build-embedded → fat → package-data)
###############################################################################
log_step "Embed pipeline: download → build-embedded → fat → package-data"

if [ "$(uname -s)" = Darwin ]; then
    log_info "macOS host: static crun build runs inside edgelet-embed-ci Docker image"
    docker build -f build/Dockerfile.embedded -t edgelet-embed-ci "${REPO_ROOT}"
    docker run --rm -v "${REPO_ROOT}:/src" -w /src edgelet-embed-ci \
        bash -c "chmod +x scripts/* 2>/dev/null || true; ARCH=${TARGET_ARCH} ./scripts/download && ARCH=${TARGET_ARCH} ./scripts/build-embedded && ARCH=${TARGET_ARCH} ./scripts/build-edgelet fat && ARCH=${TARGET_ARCH} ./scripts/package-data"
else
    run_embed_pipeline
fi

log_ok "Embedded zstd bundle packaged (fat runtime in bin/edgelet)"

###############################################################################
# Step 2: Cross-compile thin wrapper (CGO=0, embeds zstd tar)
###############################################################################
log_step "Cross-compiling edgelet thin wrapper (CGO=0)"
ARCH="${TARGET_ARCH}" EDGELET_CI_ARCHES="${TARGET_ARCH}" ./scripts/build-edgelet

EDGELET_BIN="${REPO_ROOT}/build/edgelet-linux-${TARGET_ARCH}"
[[ -f "${EDGELET_BIN}" ]] || die "Expected binary not found: ${EDGELET_BIN}"

log_ok "Binary: build/edgelet-linux-${TARGET_ARCH}"

###############################################################################
# Done
###############################################################################
log_success "Build complete for linux/${TARGET_ARCH}"
echo ""
echo "  Binary: build/edgelet-linux-${TARGET_ARCH}"
