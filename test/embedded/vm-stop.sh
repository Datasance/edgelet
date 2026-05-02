#!/usr/bin/env bash
# test/embedded/vm-stop.sh
#
# Stops (and optionally deletes) the iofog-test Lima VM.
#
# Usage:
#   ./test/embedded/vm-stop.sh [--vm-name=iofog-test] [--delete]

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/lib/log.sh"

VM_NAME="iofog-test"
DELETE=false

for arg in "$@"; do
    case "${arg}" in
        --vm-name=*) VM_NAME="${arg#*=}" ;;
        --delete)    DELETE=true ;;
    esac
done

if ! command -v limactl &>/dev/null; then
    log_warn "limactl not found — nothing to stop"
    exit 0
fi

# Lima 2.x outputs JSONL (one object per line), not a JSON array.
VM_STATUS=$(limactl list --json 2>/dev/null \
    | jq -r "select(.name == \"${VM_NAME}\") | .status" 2>/dev/null \
    | head -1 || echo "")

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
