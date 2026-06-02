#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/lib/log.sh"

SKIP_SETUP=false
SKIP_BUILD=false
SKIP_START=false
SWITCH=""
VM_NAME="edgelet-engine-lifecycle"

for arg in "$@"; do
    case "${arg}" in
        --skip-setup) SKIP_SETUP=true ;;
        --skip-build) SKIP_BUILD=true ;;
        --skip-start) SKIP_START=true ;;
        --switch=*) SWITCH="${arg#*=}" ;;
        --vm-name=*) VM_NAME="${arg#*=}" ;;
    esac
done

[[ "${SKIP_SETUP}" == false ]] && "${SCRIPT_DIR}/setup.sh"
[[ "${SKIP_BUILD}" == false ]] && "${SCRIPT_DIR}/build.sh"
[[ "${SKIP_START}" == false ]] && "${SCRIPT_DIR}/vm-start.sh" --vm-name="${VM_NAME}"

if [[ -n "${SWITCH}" ]]; then
    "${SCRIPT_DIR}/engine-switch-test.sh" --vm-name="${VM_NAME}" --switch="${SWITCH}"
else
    "${SCRIPT_DIR}/engine-switch-test.sh" --vm-name="${VM_NAME}" --switch=edgelet-to-docker
    "${SCRIPT_DIR}/engine-switch-test.sh" --vm-name="${VM_NAME}" --switch=docker-to-edgelet
fi

log_success "Engine lifecycle IT complete"
