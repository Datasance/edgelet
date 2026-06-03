#!/usr/bin/env bash
# T11-D — embedded split: restart edgelet-containerd; MS stop then reconcile.
#
# Usage:
#   ./test/workload-continuity/embedded-runtime-restart.sh [--vm-name=iofog-test]

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
# shellcheck source=test/embedded/lib/log.sh
source "${REPO_ROOT}/test/embedded/lib/log.sh"
# shellcheck source=test/workload-continuity/lib/ensure-vm.sh
source "${SCRIPT_DIR}/lib/ensure-vm.sh"

VM_NAME="iofog-test"
for arg in "$@"; do
    case "${arg}" in --vm-name=*) VM_NAME="${arg#*=}" ;; esac
done

command -v limactl >/dev/null || die "limactl required"

R() { wc_remote "${VM_NAME}" "$*"; }

log_step "T11-D data-plane restart stops MS then reconciles"

wc_embedded_split_gate "${VM_NAME}" || die "embedded split gate failed — run run-all.sh or ensure install.sh split + MS"

BEFORE_COUNT="$(R "ctr --address /run/edgelet/containerd.sock -n k8s.io containers list -q 2>/dev/null | wc -l | tr -d ' '")"
[[ "${BEFORE_COUNT}" -gt 0 ]] || die "No CRI containers — deploy an MS before T11-D"

R "systemctl restart edgelet-containerd"
sleep 5

MID_COUNT="$(R "ctr --address /run/edgelet/containerd.sock -n k8s.io containers list -q 2>/dev/null | wc -l | tr -d ' '")"
if [[ "${MID_COUNT}" -eq 0 ]]; then
    log_ok "CRI containers stopped during data-plane restart"
    (( TESTS_PASSED++ )) || true
else
    log_fail "expected 0 CRI containers mid-restart, got ${MID_COUNT}"
    (( TESTS_FAILED++ )) || true
fi

R "systemctl restart edgelet"
for i in $(seq 1 60); do
    if R "systemctl is-active --quiet edgelet" \
        && R "ctr --address /run/edgelet/containerd.sock -n k8s.io containers list -q 2>/dev/null | grep -q ."; then
        break
    fi
    sleep 3
done

AFTER_COUNT="$(R "ctr --address /run/edgelet/containerd.sock -n k8s.io containers list -q 2>/dev/null | wc -l | tr -d ' '")"
if [[ "${AFTER_COUNT}" -gt 0 ]]; then
    log_ok "MS reconciled after data-plane restart (${AFTER_COUNT} container(s))"
    (( TESTS_PASSED++ )) || true
else
    log_fail "MS not reconciled after data-plane restart"
    (( TESTS_FAILED++ )) || true
fi

print_summary
