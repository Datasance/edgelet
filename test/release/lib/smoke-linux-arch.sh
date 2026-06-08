#!/usr/bin/env bash
# test/release/lib/smoke-linux-arch.sh
#
# Release bar for Tier-1 linux/arm and linux/riscv64: daemon start, version, CRI socket.
# Uses Docker QEMU when the host cannot run the target arch natively.
#
# Usage (via wrapper):
#   ./test/release/lib/smoke-linux-arch.sh arm [linux/arm/v7]
#   ./test/release/lib/smoke-linux-arch.sh riscv64 [linux/riscv64]

set -euo pipefail

ARCH="${1:?arch required (arm|riscv64)}"
DOCKER_PLATFORM="${2:-}"

case "${ARCH}" in
    arm)
        DOCKER_PLATFORM="${DOCKER_PLATFORM:-linux/arm/v7}"
        ;;
    riscv64)
        DOCKER_PLATFORM="${DOCKER_PLATFORM:-linux/riscv64}"
        ;;
    *)
        die "unsupported arch: ${ARCH} (expected arm or riscv64)"
        ;;
esac

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../../.." && pwd)"
# shellcheck source=test/embedded/lib/log.sh
source "${REPO_ROOT}/test/embedded/lib/log.sh"

BIN="${REPO_ROOT}/build/edgelet-linux-${ARCH}"
CONFIG="${REPO_ROOT}/packaging/edgelet/etc/edgelet/config.default.yaml"

[[ -f "${BIN}" ]] || die "Missing ${BIN}; run: ./test/release/build-all.sh"
[[ -f "${CONFIG}" ]] || die "Missing ${CONFIG}"

native_smoke() {
    log_step "Release smoke linux/${ARCH} (native host)"
    # shellcheck source=test/init/lib/stage-install-bundle.sh
    source "${REPO_ROOT}/test/init/lib/stage-install-bundle.sh"
    validate_install_bundle_sources "${REPO_ROOT}"

    systemctl stop edgelet edgelet-containerd 2>/dev/null || true
    systemctl reset-failed edgelet edgelet-containerd 2>/dev/null || true
    "${REPO_ROOT}/install.sh" \
        --bin-path="${BIN}" \
        --version="dev-release-smoke" \
        --arch="${ARCH}" \
        --container-engine=edgelet

    local _elapsed=0 _timeout=180
    while (( _elapsed < _timeout )); do
        if systemctl is-active --quiet edgelet-containerd \
            && systemctl is-active --quiet edgelet \
            && test -S /run/edgelet/containerd.sock; then
            break
        fi
        sleep 2
        _elapsed=$(( _elapsed + 2 ))
    done

    assert_ok "edgelet-containerd.service active" systemctl is-active --quiet edgelet-containerd
    assert_ok "edgelet.service active (daemon)" systemctl is-active --quiet edgelet
    assert_ok "edgelet version" sh -c 'edgelet version | grep -q .'
    assert_ok "CRI socket /run/edgelet/containerd.sock" test -S /run/edgelet/containerd.sock
    print_summary
}

docker_smoke() {
    command -v docker >/dev/null || die "docker required for linux/${ARCH} smoke on this host"

    if ! docker run --rm --platform "${DOCKER_PLATFORM}" alpine:3.22 uname -m >/dev/null 2>&1; then
        die "Cannot run ${DOCKER_PLATFORM} containers — enable QEMU/binfmt (Docker Desktop or: docker run --privileged --rm tonistiigi/binfmt --install all)"
    fi

    SMOKE_CONTAINER="edgelet-release-smoke-${ARCH}"
    local _lib_vol="edgelet-release-smoke-${ARCH}-lib"
    local _ctd_vol="edgelet-release-smoke-${ARCH}-ctd"
    local _etc_vol="edgelet-release-smoke-${ARCH}-etc"
    local _timeout=420

    cleanup() {
        docker rm -f "${SMOKE_CONTAINER}" >/dev/null 2>&1 || true
    }
    trap cleanup EXIT

    docker rm -f "${SMOKE_CONTAINER}" >/dev/null 2>&1 || true

    log_step "Release smoke linux/${ARCH} (Docker ${DOCKER_PLATFORM})"

    docker run -d --name "${SMOKE_CONTAINER}" \
        --platform "${DOCKER_PLATFORM}" \
        --privileged \
        --cgroupns=host \
        -v "${BIN}:/opt/edgelet/bin:ro" \
        -v "${CONFIG}:/opt/edgelet/config.yaml:ro" \
        -v "${_lib_vol}:/var/lib/edgelet" \
        -v "${_ctd_vol}:/var/lib/edgelet-containerd" \
        -v "${_etc_vol}:/etc/edgelet" \
        -v /sys/fs/cgroup:/sys/fs/cgroup:rw \
        -e EDGELET_DAEMON=container \
        ubuntu:24.04 \
        bash -c '
set -euo pipefail
export DEBIAN_FRONTEND=noninteractive
apt-get update -qq
apt-get install -y -qq ca-certificates kmod >/dev/null
modprobe overlay 2>/dev/null || true
modprobe br_netfilter 2>/dev/null || true
mkdir -p /etc/edgelet /var/log/edgelet /run/edgelet
install -m 755 /opt/edgelet/bin /usr/local/bin/edgelet
install -m 640 /opt/edgelet/config.yaml /etc/edgelet/config.yaml
exec env EDGELET_DAEMON=container /usr/local/bin/edgelet daemon
' >/dev/null

    local _elapsed=0 _ready=false
    while (( _elapsed < _timeout )); do
        if docker exec "${SMOKE_CONTAINER}" test -S /run/edgelet/containerd.sock 2>/dev/null; then
            _ready=true
            break
        fi
        if ! docker inspect -f '{{.State.Running}}' "${SMOKE_CONTAINER}" 2>/dev/null | grep -q true; then
            log_fail "container exited before CRI socket was ready"
            docker logs "${SMOKE_CONTAINER}" 2>&1 | tail -60 >&2 || true
            if docker logs "${SMOKE_CONTAINER}" 2>&1 | grep -q 'qemu: uncaught target signal'; then
                die "embedded engine crashed under QEMU (${DOCKER_PLATFORM}) — run this smoke on Linux with binfmt or on native ${ARCH} hardware"
            fi
            die "edgelet daemon failed for linux/${ARCH}"
        fi
        sleep 3
        _elapsed=$(( _elapsed + 3 ))
    done

    if [[ "${_ready}" != true ]]; then
        log_fail "CRI socket not ready after ${_timeout}s"
        docker logs "${SMOKE_CONTAINER}" 2>&1 | tail -60 >&2 || true
        if docker logs "${SMOKE_CONTAINER}" 2>&1 | grep -q 'qemu: uncaught target signal'; then
            die "embedded engine crashed under QEMU (${DOCKER_PLATFORM}) — run this smoke on Linux with binfmt or on native ${ARCH} hardware"
        fi
        die "embedded engine did not expose /run/edgelet/containerd.sock"
    fi

    assert_ok "edgelet daemon running" \
        docker exec "${SMOKE_CONTAINER}" pgrep -f '[e]dgelet.*daemon'
    assert_ok "edgelet version" \
        sh -c "docker exec \"${SMOKE_CONTAINER}\" edgelet version | grep -q ."
    assert_ok "CRI socket /run/edgelet/containerd.sock" \
        docker exec "${SMOKE_CONTAINER}" test -S /run/edgelet/containerd.sock

    trap - EXIT
    cleanup
    trap - EXIT

    print_summary
}

use_native=false
if [[ "$(uname -s)" == Linux && "$(id -u)" -eq 0 ]]; then
    case "${ARCH}:${$(uname -m)}" in
        arm:armv7l|arm:armv6l|arm:arm) use_native=true ;;
        riscv64:riscv64) use_native=true ;;
    esac
fi

if [[ "${use_native}" == true ]]; then
    native_smoke
else
    docker_smoke
fi
