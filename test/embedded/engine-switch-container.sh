#!/usr/bin/env bash
# test/embedded/engine-switch-container.sh
#
# Nested edgelet-linux container: deploy on edgelet engine, cold-switch to docker,
# docker restart, verify MS reconciles on host Docker.
#
# Usage:
#   EDGELET_IMAGE=edgelet-linux:local ./test/embedded/engine-switch-container.sh
#
# Prerequisites: docker CLI, jq, locally built/pulled edgelet-linux image.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
source "${SCRIPT_DIR}/lib/log.sh"

IMAGE="${EDGELET_IMAGE:-edgelet-linux:local}"
NAME="${EDGELET_CONTAINER_NAME:-edgelet-engine-switch-smoke}"
LIB_VOL="${EDGELET_LIB_VOL:-edgelet-engine-switch-lib}"
ETC_VOL="${EDGELET_ETC_VOL:-edgelet-engine-switch-etc}"
MS_NAME="engine-switch-ms"

cleanup() {
    docker rm -f "${NAME}" >/dev/null 2>&1 || true
}
trap cleanup EXIT

if ! command -v docker >/dev/null || ! command -v jq >/dev/null; then
    die "docker and jq required"
fi

log_step "Starting privileged nested edgelet (${IMAGE}) with host docker.sock"
docker run -d --name "${NAME}" --privileged --net=host \
    -v "${LIB_VOL}:/var/lib/edgelet" \
    -v "${ETC_VOL}:/etc/edgelet" \
    -v /var/run/docker.sock:/var/run/docker.sock:rw \
    "${IMAGE}" >/dev/null

wait_ready() {
    local i
    for i in $(seq 1 30); do
        if docker exec "${NAME}" edgelet system status -o json 2>/dev/null | jq -e '.["runtime.engine"]' >/dev/null; then
            return 0
        fi
        sleep 2
    done
    return 1
}

assert_ok "daemon becomes ready" wait_ready

log_step "Deploy microservice on edgelet engine"
docker cp "${REPO_ROOT}/test/engine-lifecycle/fixtures/engine-switch-ms.yaml" \
    "${NAME}:/tmp/engine-switch-ms.yaml"
assert_contains "MS deploy succeeds" "applied successfully" \
    docker exec "${NAME}" edgelet deploy -f /tmp/engine-switch-ms.yaml
assert_contains "MS running before switch" "running" \
    docker exec "${NAME}" edgelet ms ls

log_step "Cold switch edgelet -> docker"
assert_contains "pendingRestart false before switch" "false" \
    sh -c "docker exec \"${NAME}\" edgelet system status -o json | jq -r '.[\"runtime.pendingRestart\"]'"
docker exec "${NAME}" edgelet config --container-engine docker
assert_contains "pendingRestart true after config change" "true" \
    sh -c "docker exec \"${NAME}\" edgelet system status -o json | jq -r '.[\"runtime.pendingRestart\"]'"

log_step "Container restart (equivalent to systemctl restart edgelet)"
docker restart "${NAME}" >/dev/null
sleep 5
assert_ok "daemon survives restart" wait_ready

assert_contains "pendingRestart cleared" "false" \
    sh -c "docker exec \"${NAME}\" edgelet system status -o json | jq -r '.[\"runtime.pendingRestart\"]'"
assert_contains "runtime.engine is docker" "docker" \
    sh -c "docker exec \"${NAME}\" edgelet system status -o json | jq -r '.[\"runtime.engine\"]'"

log_step "Wait for MS reconcile on docker engine"
for i in $(seq 1 30); do
    if docker exec "${NAME}" edgelet ms ls 2>/dev/null | grep -q "${MS_NAME}.*running"; then
        break
    fi
    sleep 2
done
assert_contains "MS running after switch" "running" \
    docker exec "${NAME}" edgelet ms ls

assert_ok "MS container visible on host docker" \
    sh -c "docker ps --format '{{.Names}}' | grep -q ."

if (( TESTS_FAILED > 0 )); then
    die "${TESTS_FAILED} assertion(s) failed"
fi
log_success "Nested container engine-switch smoke passed"
