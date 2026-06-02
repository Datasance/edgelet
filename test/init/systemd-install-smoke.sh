#!/usr/bin/env bash
# T10-A — systemd install smoke (Lima iofog-test VM or native linux root).
#
# Usage (macOS):
#   ./test/embedded/build.sh
#   ./test/embedded/vm-start.sh
#   ./test/init/systemd-install-smoke.sh
#
# Usage (linux root):
#   sudo ./test/init/systemd-install-smoke.sh --native

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
# shellcheck source=test/embedded/lib/log.sh
source "${REPO_ROOT}/test/embedded/lib/log.sh"
# shellcheck source=test/init/lib/stage-install-bundle.sh
source "${SCRIPT_DIR}/lib/stage-install-bundle.sh"

VM_NAME="iofog-test"
NATIVE=false
HOST_ARCH="$(native_arch)"
[[ "${HOST_ARCH}" == "arm64" ]] && TARGET_ARCH="arm64" || TARGET_ARCH="amd64"

for arg in "$@"; do
    case "${arg}" in
        --vm-name=*) VM_NAME="${arg#*=}" ;;
        --native)    NATIVE=true ;;
        -h|--help)
            echo "Usage: $0 [--vm-name=NAME] [--native]"
            exit 0
            ;;
    esac
done

BIN="${REPO_ROOT}/build/edgelet-linux-${TARGET_ARCH}"
INSTALL_SH="${REPO_ROOT}/install.sh"
[[ -f "${BIN}" ]] || die "Missing ${BIN}; run: make build-linux-${TARGET_ARCH}"
validate_install_bundle_sources "${REPO_ROOT}"

run_remote() {
    if [[ "${NATIVE}" == true ]]; then
        sudo bash -c "$*"
        return
    fi
    command -v limactl >/dev/null || die "limactl required (or use --native on linux)"
    echo "$*" | limactl --tty=false shell "${VM_NAME}" -- sudo bash
}

log_step "T10-A systemd install smoke (vm=${VM_NAME} native=${NATIVE})"

if [[ "${NATIVE}" == true ]]; then
    log_info "Running install.sh from repo root (native)..."
    sudo "${INSTALL_SH}" --bin-path="${BIN}" --version=dev-t10a --arch="${TARGET_ARCH}" --container-engine=edgelet
else
    SSH_CONFIG="${HOME}/.lima/${VM_NAME}/ssh.config"
    SSH_HOST="lima-${VM_NAME}"
    STAGE="/tmp/edgelet-t10a"
    log_info "Staging install bundle to ${STAGE} (binary, install.sh, scripts/lib, packaging/init)..."
    stage_install_bundle_ssh "${SSH_CONFIG}" "${SSH_HOST}" "${STAGE}" "${REPO_ROOT}" "${BIN}"
    run_remote "
        set -e
        systemctl stop edgelet 2>/dev/null || true
        systemctl reset-failed edgelet 2>/dev/null || true
        chmod +x ${STAGE}/install.sh ${STAGE}/edgelet ${STAGE}/scripts/edgelet-shutdown
        ${STAGE}/install.sh --bin-path=${STAGE}/edgelet --version=dev-t10a --arch=${TARGET_ARCH} --container-engine=edgelet
    "
fi

wait_for_service_active() {
    local _timeout="${1:-90}" _elapsed=0
    while (( _elapsed < _timeout )); do
        if run_remote "systemctl is-active --quiet edgelet"; then
            return 0
        fi
        sleep 2
        _elapsed=$(( _elapsed + 2 ))
    done
    run_remote "journalctl -u edgelet -n 20 --no-pager" || true
    return 1
}

wait_for_service_active 90 || die "edgelet.service did not become active"

assert_ok() {
    local msg="$1"
    shift
    if "$@"; then
        log_ok "${msg}"
    else
        die "${msg}"
    fi
}

assert_ok "edgelet.service is active" \
    run_remote "systemctl is-active --quiet edgelet"

assert_ok "DelegateSubgroup configured" \
    run_remote "systemctl show edgelet -p DelegateSubgroup --value | grep -q supervisor"

assert_ok "ExecStop uses edgelet-shutdown" \
    run_remote "grep -q edgelet-shutdown /etc/systemd/system/edgelet.service"

assert_ok "edgelet-shutdown installed" \
    run_remote "test -x /usr/libexec/edgelet/edgelet-shutdown"

log_info "Waiting for edgelet API (up to 180s)..."
if [[ "${NATIVE}" == true ]]; then
    wait_edgelet_api_ready 'sudo bash -c "test -S /run/edgelet/edgelet.sock && edgelet system status -o json | jq -e .cgroupMode >/dev/null"' 180
else
    wait_edgelet_api_ready "limactl --tty=false shell ${VM_NAME} -- sudo bash -c 'test -S /run/edgelet/edgelet.sock && edgelet system status -o json | jq -e .cgroupMode >/dev/null'" 180
fi

assert_ok "cgroup status reachable" \
    run_remote "edgelet system status -o json | jq -e '.cgroupMode' >/dev/null"

assert_ok "cgroup driver policy (systemd or cgroupfs)" \
    run_remote 'set -e
driver=$(edgelet system status -o json | jq -r .cgroupDriver)
case "${driver}" in systemd|cgroupfs) ;; *) echo "unexpected driver=${driver}"; exit 1 ;; esac'

log_success "T10-A systemd install smoke passed"
