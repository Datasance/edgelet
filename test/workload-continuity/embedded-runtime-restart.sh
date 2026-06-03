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

# cri_container_count — all k8s.io ctr entries (pause + workload per pod).
cri_container_count() {
    R "ctr --address /run/edgelet/containerd.sock -n k8s.io containers list -q 2>/dev/null | wc -l | tr -d ' '"
}

# wait_cri_drained TIMEOUT — poll until drain completes (stale pause/workload rows gone).
wait_cri_drained() {
    local _timeout="${1:-30}" _elapsed=0 _count=""
    while (( _elapsed < _timeout )); do
        _count="$(cri_container_count)"
        if [[ "${_count}" == "0" ]]; then
            return 0
        fi
        sleep 1
        _elapsed=$(( _elapsed + 1 ))
    done
    return 1
}

log_step "T11-D data-plane restart stops MS then reconciles"

wc_embedded_split_gate "${VM_NAME}" || die "embedded split gate failed — run run-all.sh or ensure install.sh split + MS"

BEFORE_COUNT="$(cri_container_count)"
[[ "${BEFORE_COUNT}" -gt 0 ]] || die "No CRI containers — deploy an MS before T11-D"

R "systemctl restart edgelet-containerd"

if wait_cri_drained 30; then
    log_ok "CRI containers drained after edgelet-containerd restart"
    (( TESTS_PASSED++ )) || true
else
    log_fail "CRI containers not drained within 30s (count=$(cri_container_count))"
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

AFTER_COUNT="$(cri_container_count)"
if [[ "${AFTER_COUNT}" -gt 0 ]]; then
    log_ok "MS reconciled after data-plane restart (${AFTER_COUNT} container(s))"
    (( TESTS_PASSED++ )) || true
else
    log_fail "MS not reconciled after data-plane restart"
    (( TESTS_FAILED++ )) || true
fi

print_summary
