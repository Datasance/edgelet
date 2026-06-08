#!/usr/bin/env bash
# embedded control restart — embedded split: restart edgelet only; CRI workloads survive.
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

log_step "embedded control restart embedded control restart leaves CRI workloads running"

wc_embedded_split_gate "${VM_NAME}" || die "embedded split gate failed — run run-all.sh or ensure install.sh split + MS"

assert_ok "edgelet-containerd active" \
    R "systemctl is-active --quiet edgelet-containerd"

assert_ok "edgelet active" \
    R "systemctl is-active --quiet edgelet"

BEFORE="$(wc_cri_container_ids "${VM_NAME}")"
[[ -n "${BEFORE// /}" ]] || die "No CRI containers — deploy an MS before embedded control restart"
log_info "CRI containers before: ${BEFORE}"

R "systemctl restart edgelet"

for i in $(seq 1 30); do
    if R "systemctl is-active --quiet edgelet"; then
        break
    fi
    sleep 2
done

assert_ok "edgelet active after restart" \
    R "systemctl is-active --quiet edgelet"

wc_wait_embedded_control_ready "${VM_NAME}" 120 \
    || die "embedded control not ready after restart (containerd/socket/API)"

assert_ok "edgelet-containerd still active (data plane)" \
    R "systemctl is-active --quiet edgelet-containerd"

AFTER="$(wc_wait_cri_ids_unchanged "${VM_NAME}" "${BEFORE}" 120 || true)"
if [[ "${BEFORE}" == "${AFTER}" && -n "${AFTER// /}" ]]; then
    log_ok "same CRI container IDs after control restart"
    (( TESTS_PASSED++ )) || true
else
    log_fail "CRI containers changed: before='${BEFORE}' after='${AFTER:-<none>}'"
    (( TESTS_FAILED++ )) || true
fi

print_summary
