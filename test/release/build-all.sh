#!/usr/bin/env bash
# test/release/build-all.sh
#
# Build all 7 release binaries (R96 / Plan 15-1).
#
# Linux amd64/arm64/arm/riscv64: embed pipeline + thin wrapper (Docker edgelet-embed-ci on macOS).
# Desktop: darwin amd64+arm64 and windows amd64 cross-compile.
# Size gate: scripts/binary_size_check.sh for linux amd64 and arm64.
#
# Output:
#   build/edgelet-linux-{amd64,arm64,arm,riscv64}
#   build/edgelet-darwin-{amd64,arm64}
#   build/edgelet-windows-amd64.exe
#
# Usage:
#   ./test/release/build-all.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
# shellcheck source=test/embedded/lib/log.sh
source "${REPO_ROOT}/test/embedded/lib/log.sh"

LINUX_ARCHES=(amd64 arm64 arm riscv64)
SIZE_GATE_ARCHES=(amd64 arm64)

RELEASE_BINARIES=(
    edgelet-linux-amd64
    edgelet-linux-arm64
    edgelet-linux-arm
    edgelet-linux-riscv64
    edgelet-darwin-amd64
    edgelet-darwin-arm64
    edgelet-windows-amd64.exe
)

chmod +x "${REPO_ROOT}/scripts/download" "${REPO_ROOT}/scripts/download-root" \
    "${REPO_ROOT}/scripts/stage-root-aux" "${REPO_ROOT}/scripts/build-crun-static" \
    "${REPO_ROOT}/scripts/build-embedded" "${REPO_ROOT}/scripts/package-data" \
    "${REPO_ROOT}/scripts/build-edgelet" "${REPO_ROOT}/scripts/check-embed-static.sh" \
    "${REPO_ROOT}/scripts/binary_size_check.sh" 2>/dev/null || true

run_embed_pipeline() {
    local _arch="$1"
    ARCH="${_arch}" ./scripts/download
    ARCH="${_arch}" ./scripts/build-embedded
    ARCH="${_arch}" ./scripts/build-edgelet fat
    ARCH="${_arch}" ./scripts/package-data
}

docker_platform() {
    case "$1" in
        amd64) echo linux/amd64 ;;
        arm64) echo linux/arm64 ;;
        arm) echo linux/arm/v7 ;;
        riscv64) echo linux/riscv64 ;;
        *) die "unsupported linux arch: $1" ;;
    esac
}

# Dockerfile.embedded --ci profile supports amd64 and arm64 builders only.
# arm and riscv64 embed builds cross-compile on the amd64 CI image (release.yml pattern).
docker_build_platform() {
    case "$1" in
        amd64|arm|riscv64) echo linux/amd64 ;;
        arm64) echo linux/arm64 ;;
        *) die "unsupported linux arch: $1" ;;
    esac
}

embed_ci_image() {
    case "$1" in
        arm64) echo edgelet-embed-ci-arm64 ;;
        amd64|arm|riscv64) echo edgelet-embed-ci-amd64 ;;
        *) die "unsupported linux arch: $1" ;;
    esac
}

ensure_embed_ci_image() {
    local _arch="$1"
    local _platform _tag
    _platform="$(docker_build_platform "${_arch}")"
    _tag="$(embed_ci_image "${_arch}")"
    if [[ "${RELEASE_FRESH_CI_IMAGE:-}" != 1 ]] && docker image inspect "${_tag}" >/dev/null 2>&1; then
        log_info "Reusing Docker image ${_tag} (${_platform})"
        return 0
    fi
    log_info "Building Docker image ${_tag} (${_platform})"
    docker build --platform "${_platform}" -f build/Dockerfile.embedded -t "${_tag}" "${REPO_ROOT}"
}

run_embed_pipeline_docker() {
    local _arch="$1"
    local _platform _tag
    _platform="$(docker_build_platform "${_arch}")"
    _tag="$(embed_ci_image "${_arch}")"
    ensure_embed_ci_image "${_arch}"
    docker run --rm --platform "${_platform}" -v "${REPO_ROOT}:/src" -w /src "${_tag}" \
        bash -c "chmod +x scripts/* 2>/dev/null || true; \
            ARCH=${_arch} ./scripts/download && \
            ARCH=${_arch} ./scripts/build-embedded && \
            ARCH=${_arch} ./scripts/build-edgelet fat && \
            ARCH=${_arch} ./scripts/package-data && \
            ARCH=${_arch} ./scripts/check-embed-static.sh build/bin/edgelet build/stage/bin"
}

build_linux_arch() {
    local _arch="$1"
    log_step "Linux ${_arch}: embed pipeline + thin wrapper"
    cd "${REPO_ROOT}"

    if [[ "$(uname -s)" == Darwin ]]; then
        run_embed_pipeline_docker "${_arch}"
    else
        run_embed_pipeline "${_arch}"
        ARCH="${_arch}" ./scripts/check-embed-static.sh build/bin/edgelet build/stage/bin
    fi

    log_step "Linux ${_arch}: cross-compile thin wrapper (CGO=0)"
    ARCH="${_arch}" EDGELET_CI_ARCHES="${_arch}" ./scripts/build-edgelet

    local _bin="${REPO_ROOT}/build/edgelet-linux-${_arch}"
    [[ -f "${_bin}" ]] || die "Expected binary not found: ${_bin}"
    log_ok "build/edgelet-linux-${_arch}"
}

log_step "Release build matrix — 7 binaries (Plan 15 / R96)"
log_info "Repository root: ${REPO_ROOT}"

cd "${REPO_ROOT}"

if [[ "$(uname -s)" == Darwin ]]; then
    command -v docker >/dev/null || die "Docker required on macOS for linux embed builds"
fi

for _arch in "${LINUX_ARCHES[@]}"; do
    build_linux_arch "${_arch}"
done

log_step "Binary size gate (linux amd64 + arm64)"
for _arch in "${SIZE_GATE_ARCHES[@]}"; do
    ARCH="${_arch}" ./scripts/binary_size_check.sh
done

log_step "Desktop cross-compile (darwin amd64+arm64, windows amd64)"
make -C "${REPO_ROOT}" build-desktop-darwin build-desktop-windows

log_step "Verify release filenames"
for _name in "${RELEASE_BINARIES[@]}"; do
    [[ -f "${REPO_ROOT}/build/${_name}" ]] || die "Missing release binary: build/${_name}"
done

log_success "All 7 release binaries present under build/"
echo ""
ls -lh "${REPO_ROOT}/build/edgelet-linux-"* \
    "${REPO_ROOT}/build/edgelet-darwin-"* \
    "${REPO_ROOT}/build/edgelet-windows-amd64.exe"
