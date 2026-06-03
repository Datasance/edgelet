#!/usr/bin/env bash
# T11-C — embedded split: restart edgelet only; CRI workloads survive.
#
# Usage:
#   ./test/workload-continuity/embedded-restart.sh [--vm-name=iofog-test]

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

log_step "T11-C embedded control restart leaves CRI workloads running"

wc_embedded_split_gate "${VM_NAME}" || die "embedded split gate failed — run run-all.sh or ensure install.sh split + MS"

assert_ok "edgelet-containerd active" \
    R "systemctl is-active --quiet edgelet-containerd"

assert_ok "edgelet active" \
    R "systemctl is-active --quiet edgelet"

BEFORE="$(wc_cri_container_ids "${VM_NAME}")"
[[ -n "${BEFORE// /}" ]] || die "No CRI containers — deploy an MS before T11-C"
log_info "CRI containers before: ${BEFORE}"

R "systemctl restart edgelet"

for i in $(seq 1 30); do
    if R "systemctl is-active --quiet edgelet"; then
        break
    fi
    sleep 2
done

assert_ok "edgelet-containerd still active (data plane)" \
    R "systemctl is-active --quiet edgelet-containerd"

AFTER="$(wc_cri_container_ids "${VM_NAME}")"
if [[ "${BEFORE}" == "${AFTER}" ]]; then
    log_ok "same CRI container IDs after control restart"
    (( TESTS_PASSED++ )) || true
else
    log_fail "CRI containers changed: before='${BEFORE}' after='${AFTER}'"
    (( TESTS_FAILED++ )) || true
fi

print_summary
