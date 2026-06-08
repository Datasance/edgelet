#!/usr/bin/env bash
# test/init/vm-stop-alpine.sh — Stop (and optionally delete) edgelet-openrc Lima VM.
#
# Usage:
#   ./test/init/vm-stop-alpine.sh [--vm-name=edgelet-openrc] [--delete]

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
# shellcheck source=test/embedded/lib/log.sh
source "${REPO_ROOT}/test/embedded/lib/log.sh"
# shellcheck source=test/init/lib/lima.sh
source "${SCRIPT_DIR}/lib/lima.sh"

VM_NAME="edgelet-openrc"
DELETE=false

for arg in "$@"; do
    case "${arg}" in
        --vm-name=*) VM_NAME="${arg#*=}" ;;
        --delete)    DELETE=true ;;
        -h|--help)
            echo "Usage: $0 [--vm-name=NAME] [--delete]"
            exit 0
            ;;
    esac
done

if ! command -v limactl &>/dev/null; then
    log_warn "limactl not found — nothing to stop"
    exit 0
fi

VM_STATUS="$(lima_vm_status "${VM_NAME}")"
if [[ -z "${VM_STATUS}" ]]; then
    log_info "VM '${VM_NAME}' does not exist — nothing to do"
    exit 0
fi

if [[ "${VM_STATUS}" == "Running" ]]; then
    log_step "Stopping VM '${VM_NAME}'"
    limactl stop "${VM_NAME}"
    log_ok "VM stopped"
else
    log_info "VM '${VM_NAME}' is not running (status: ${VM_STATUS})"
fi

if [[ "${DELETE}" == "true" ]]; then
    log_step "Deleting VM '${VM_NAME}'"
    limactl delete "${VM_NAME}"
    log_ok "VM deleted"
fi
