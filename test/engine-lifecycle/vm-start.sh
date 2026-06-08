#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/lib/log.sh"

VM_NAME="edgelet-engine-lifecycle"
TIMEOUT=300
for arg in "$@"; do
    case "${arg}" in
        --vm-name=*) VM_NAME="${arg#*=}" ;;
        --timeout=*) TIMEOUT="${arg#*=}" ;;
    esac
done

LIMA_YAML="${SCRIPT_DIR}/lima-ubuntu-docker.yaml"
command -v limactl >/dev/null || die "limactl not found — run setup.sh"

vm_status() {
    limactl list --json 2>/dev/null \
        | jq -r "select(.name == \"${VM_NAME}\") | .status" 2>/dev/null \
        | head -1 || true
}

if [[ -z "$(vm_status)" ]]; then
    log_step "Creating VM ${VM_NAME}"
    limactl create --name="${VM_NAME}" "${LIMA_YAML}"
fi
if [[ "$(vm_status)" != "Running" ]]; then
    limactl start --timeout=1200s "${VM_NAME}"
fi

elapsed=0
until limactl --tty=false shell "${VM_NAME}" -- systemctl is-active docker.service &>/dev/null; do
    (( elapsed >= TIMEOUT )) && die "Docker not active within ${TIMEOUT}s"
    sleep 5
    (( elapsed += 5 )) || true
done
log_success "VM ${VM_NAME} ready with Docker active"
