#!/usr/bin/env bash
# embedded CP apply + controller status API + CP lifecycle (unprovisioned) + CP DNS resolution on embedded engine (iofog-test Lima VM).
#
# Usage: ./test/control-plane/t12-embedded.sh [--vm-name=iofog-test] [--skip-setup]

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=test/control-plane/lib/cp.sh
source "${SCRIPT_DIR}/lib/cp.sh"

VM_NAME="iofog-test"
SKIP_SETUP=false
FIXTURE="${CP_DEFAULT_FIXTURE}"
TARGET_ARCH="$(wc_target_arch)"

for arg in "$@"; do
    case "${arg}" in
        --vm-name=*) VM_NAME="${arg#*=}" ;;
        --skip-setup) SKIP_SETUP=true ;;
        --fixture=*) FIXTURE="${arg#*=}" ;;
    esac
done

cp_fixture_metadata "${FIXTURE}"
log_step "embedded CP apply embedded (${VM_NAME}) — ControlPlane ${CP_NS}/${CP_NAME}"

cp_ensure_embedded_vm "${CP_REPO_ROOT}" "${VM_NAME}" "${TARGET_ARCH}" "${SKIP_SETUP}"
cp_deploy "${VM_NAME}" "${FIXTURE}"
cp_wait_running "${VM_NAME}"
cp_assert_deployed "${VM_NAME}"

log_step "controller status API controller /api/v3/status"
cp_assert_status_api "${VM_NAME}"

log_step "CP DNS resolution DNS (fixture namespace/name FQDNs)"
cp_assert_dns "${VM_NAME}"

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
