#!/usr/bin/env bash
# T10-B — Alpine openrc start/stop smoke (Lima edgelet-openrc profile).
#
# Prerequisites:
#   ./test/init/vm-start-alpine.sh
#   make build-linux-arm64
#
# Usage:
#   ./test/init/alpine-openrc-smoke.sh [--vm-name=edgelet-openrc]

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
# shellcheck source=test/embedded/lib/log.sh
source "${REPO_ROOT}/test/embedded/lib/log.sh"
# shellcheck source=test/init/lib/stage-install-bundle.sh
source "${SCRIPT_DIR}/lib/stage-install-bundle.sh"
# shellcheck source=test/init/lib/lima.sh
source "${SCRIPT_DIR}/lib/lima.sh"

VM_NAME="edgelet-openrc"
TARGET_ARCH="arm64"

for arg in "$@"; do
    case "${arg}" in
        --vm-name=*) VM_NAME="${arg#*=}" ;;
        -h|--help)
            echo "Usage: $0 [--vm-name=NAME]"
            exit 0
            ;;
    esac
done

BIN="${REPO_ROOT}/build/edgelet-linux-${TARGET_ARCH}"
[[ -f "${BIN}" ]] || die "Missing ${BIN}"
validate_install_bundle_sources "${REPO_ROOT}"

command -v limactl >/dev/null || die "limactl required"

if ! lima_vm_running "${VM_NAME}"; then
    log_step "VM ${VM_NAME} not running — starting via vm-start-alpine.sh"
    "${SCRIPT_DIR}/vm-start-alpine.sh" --vm-name="${VM_NAME}"
fi

run_remote() {
    echo "$*" | limactl --tty=false shell "${VM_NAME}" -- sudo bash
}

assert_openrc_pid1() {
    if ! run_remote "rc-status -s >/dev/null 2>&1"; then
        die "OpenRC supervisor is not running (rc-status failed). Recreate the VM:
  ./test/init/vm-stop-alpine.sh --delete
  ./test/init/vm-start-alpine.sh"
    fi
}

log_step "T10-B Alpine openrc smoke (${VM_NAME})"
assert_openrc_pid1

SSH_CONFIG="${HOME}/.lima/${VM_NAME}/ssh.config"
SSH_HOST="lima-${VM_NAME}"
STAGE="/tmp/edgelet-t10b"

log_info "Staging install bundle to ${STAGE}..."
stage_install_bundle_ssh "${SSH_CONFIG}" "${SSH_HOST}" "${STAGE}" "${REPO_ROOT}" "${BIN}"

run_remote "
    set -e
    chmod +x ${STAGE}/install.sh ${STAGE}/edgelet ${STAGE}/scripts/edgelet-shutdown
    ${STAGE}/install.sh --bin-path=${STAGE}/edgelet --version=dev-t10b --arch=${TARGET_ARCH} --container-engine=edgelet
"

run_remote "rc-service edgelet status" || true
run_remote "rc-service edgelet stop"
run_remote "rc-service edgelet start"
run_remote "pgrep -f '[e]dgelet daemon'"

log_success "T10-B Alpine openrc smoke passed"
