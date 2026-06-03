#!/usr/bin/env bash
# T11-A — docker leave-running: restart edgelet control plane; MS keep same container ID.
#
# Usage:
#   ./test/workload-continuity/docker-restart.sh [--vm-name=edgelet-engine-lifecycle]

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
# shellcheck source=test/embedded/lib/log.sh
source "${REPO_ROOT}/test/embedded/lib/log.sh"
# shellcheck source=test/workload-continuity/lib/ensure-vm.sh
source "${SCRIPT_DIR}/lib/ensure-vm.sh"

VM_NAME="edgelet-engine-lifecycle"
MS_NAME="workload-ms"
for arg in "$@"; do
    case "${arg}" in --vm-name=*) VM_NAME="${arg#*=}" ;; esac
done

command -v limactl >/dev/null || die "limactl required"

R() { wc_remote "${VM_NAME}" "$*"; }

log_step "T11-A docker control restart leaves MS running"

assert_ok "edgelet active before restart" \
    R "systemctl is-active --quiet edgelet"

CONTAINER_ID="$(wc_docker_ms_container_id "${VM_NAME}" "${MS_NAME}")"
[[ -n "${CONTAINER_ID}" ]] || die "No running ${MS_NAME} found — deploy an MS before T11-A"

log_info "MS container before restart: ${CONTAINER_ID}"

R "systemctl restart edgelet"

for i in $(seq 1 30); do
    if R "systemctl is-active --quiet edgelet"; then
        break
    fi
    sleep 2
done

assert_ok "edgelet active after restart" \
    R "systemctl is-active --quiet edgelet"

wc_wait_edgelet_api "${VM_NAME}" 120 || die "edgelet API not ready after restart"

AFTER_ID="$(wc_wait_docker_ms_container_id "${VM_NAME}" "${MS_NAME}" "${CONTAINER_ID}" 120 || true)"
if [[ "${CONTAINER_ID}" == "${AFTER_ID}" && -n "${AFTER_ID}" ]]; then
    log_ok "same Docker container ID after control restart (${CONTAINER_ID})"
    (( TESTS_PASSED++ )) || true
else
    log_fail "container ID changed: before=${CONTAINER_ID} after=${AFTER_ID:-<none>}"
    (( TESTS_FAILED++ )) || true
fi

print_summary
