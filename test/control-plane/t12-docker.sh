#!/usr/bin/env bash
# docker CP apply + controller status API + CP lifecycle (unprovisioned) on docker engine (edgelet-engine-lifecycle Lima VM).
#
# Usage: ./test/control-plane/t12-docker.sh [--vm-name=edgelet-engine-lifecycle] [--skip-setup]

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=test/control-plane/lib/cp.sh
source "${SCRIPT_DIR}/lib/cp.sh"

VM_NAME="edgelet-engine-lifecycle"
SKIP_SETUP=false
FIXTURE="${CP_DEFAULT_FIXTURE}"

for arg in "$@"; do
    case "${arg}" in
        --vm-name=*) VM_NAME="${arg#*=}" ;;
        --skip-setup) SKIP_SETUP=true ;;
        --fixture=*) FIXTURE="${arg#*=}" ;;
    esac
done

cp_fixture_metadata "${FIXTURE}"
log_step "docker CP apply docker (${VM_NAME}) — ControlPlane ${CP_NS}/${CP_NAME}"

cp_ensure_docker_vm "${CP_REPO_ROOT}" "${VM_NAME}" "${SKIP_SETUP}"
cp_deploy "${VM_NAME}" "${FIXTURE}"
cp_wait_running "${VM_NAME}"
cp_assert_deployed "${VM_NAME}"

log_step "controller status API controller /api/v3/status"
cp_assert_status_api "${VM_NAME}"

log_step "CP restart T21-1 edgelet controlplane restart"
cp_restart "${VM_NAME}" false

log_step "CP restart T21-2 edgelet controlplane restart --pull"
cp_restart "${VM_NAME}" true

log_step "CP restart T21-3 restartCount >= 1"
cp_assert_restart_count_min "${VM_NAME}" 1

log_step "CP lifecycle (unprovisioned) ms rm + reconcile + controlplane delete"
cp_deploy "${VM_NAME}" "${FIXTURE}"
cp_wait_running "${VM_NAME}"
cp_assert_lifecycle "${VM_NAME}"

print_summary
