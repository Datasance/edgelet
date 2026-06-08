#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
VM_NAME="edgelet-engine-lifecycle"
DELETE=false
for arg in "$@"; do
    case "${arg}" in
        --vm-name=*) VM_NAME="${arg#*=}" ;;
        --delete) DELETE=true ;;
    esac
done
if [[ "${DELETE}" == true ]]; then
    limactl delete -f "${VM_NAME}" 2>/dev/null || true
else
    limactl stop "${VM_NAME}" 2>/dev/null || true
fi
