#!/usr/bin/env bash
# test/embedded/container-deploy-smoke.sh
#
# Documented gate for nested edgelet-linux + deploy smoke (Plan 9B).
# Requires a locally built/pulled edgelet-linux image and Docker with privileged support.
#
# Usage:
#   ./test/embedded/run-all-nested-docker.sh
#   EDGELET_IMAGE=edgelet-linux:local ./test/embedded/container-deploy-smoke.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/lib/log.sh"

IMAGE="${EDGELET_IMAGE:-ghcr.io/datasance/edgelet-linux:latest}"
NAME="${EDGELET_CONTAINER_NAME:-edgelet-nested-smoke}"
LIB_VOL="${EDGELET_LIB_VOL:-edgelet-nested-smoke-lib}"
ETC_VOL="${EDGELET_ETC_VOL:-edgelet-nested-smoke-etc}"

cleanup() {
    docker rm -f "${NAME}" >/dev/null 2>&1 || true
}
trap cleanup EXIT

if ! command -v docker >/dev/null; then
    die "docker CLI required for nested deploy smoke"
fi

log_step "Starting privileged nested edgelet container (${IMAGE})"
docker run -d --name "${NAME}" --privileged \
    -v "${LIB_VOL}:/var/lib/edgelet" \
    -v "${ETC_VOL}:/etc/edgelet" \
    "${IMAGE}" >/dev/null

sleep 5

log_step "Checking cgroup bootstrap inside container"
docker exec "${NAME}" edgelet system status -o json | jq -e '.cgroupNested == "true"' >/dev/null

assert_ok "agent cgroup delegates cpu controller for nested CRI" \
    sh -c "docker exec \"${NAME}\" edgelet system status -o json | jq -e '.cgroupDelegatedControllers | test(\"cpu\")'"

log_step "Deploying microservice smoke workload"
cat >/tmp/smoke-ms.yaml <<'EOF'
apiVersion: edgelet.iofog.org/v1
kind: Microservice
metadata:
  name: nested-smoke-ms
spec:
  image: docker.io/library/alpine:3.19
  registry: 1
  container:
    hostNetworkMode: false
    commands: [/bin/sh, -lc, "sleep 300"]
  schedule: 50
EOF
docker cp /tmp/smoke-ms.yaml "${NAME}:/tmp/smoke-ms.yaml"
docker exec "${NAME}" edgelet deploy -f /tmp/smoke-ms.yaml
docker exec "${NAME}" edgelet ms ls

log_info "Nested privileged deploy smoke passed"
