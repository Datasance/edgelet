#!/usr/bin/env bash
# T10-B+ — Alpine openrc embedded runtime smoke (Plan 10-9).
#
# Requires 10-8 static embed in the thin bundle (fat ELF statically linked).
#
# Prerequisites:
#   ./test/embedded/build.sh   # or make build-linux-arm64
#   ./test/init/vm-start-alpine.sh
#   ./test/init/alpine-openrc-smoke.sh   # when run standalone
#
# Usage:
#   ./test/init/alpine-openrc-runtime-smoke.sh [--vm-name=edgelet-openrc]
#   ./test/init/alpine-openrc-runtime-smoke.sh --after-t10-b   # default from run-all.sh
#   ./test/init/alpine-openrc-runtime-smoke.sh --fresh-install

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
# shellcheck source=test/embedded/lib/log.sh
source "${REPO_ROOT}/test/embedded/lib/log.sh"
# shellcheck source=test/init/lib/stage-install-bundle.sh
source "${SCRIPT_DIR}/lib/stage-install-bundle.sh"
# shellcheck source=test/init/lib/embed-runtime.sh
source "${SCRIPT_DIR}/lib/embed-runtime.sh"
# shellcheck source=test/init/lib/lima.sh
source "${SCRIPT_DIR}/lib/lima.sh"

VM_NAME="edgelet-openrc"
TARGET_ARCH="arm64"
AFTER_T10_B=false
FRESH_INSTALL=false

for arg in "$@"; do
    case "${arg}" in
        --vm-name=*) VM_NAME="${arg#*=}" ;;
        --after-t10-b) AFTER_T10_B=true ;;
        --fresh-install) FRESH_INSTALL=true ;;
        -h|--help)
            echo "Usage: $0 [--vm-name=NAME] [--after-t10-b | --fresh-install]"
            exit 0
            ;;
    esac
done

if [[ "${FRESH_INSTALL}" == true ]]; then
    AFTER_T10_B=false
fi

BIN="${REPO_ROOT}/build/edgelet-linux-${TARGET_ARCH}"
[[ -f "${BIN}" ]] || die "Missing ${BIN}; run: ./test/embedded/build.sh or make build-linux-${TARGET_ARCH}"
validate_install_bundle_sources "${REPO_ROOT}"

command -v limactl >/dev/null || die "limactl required"

if ! lima_vm_running "${VM_NAME}"; then
    log_step "VM ${VM_NAME} not running — starting via vm-start-alpine.sh"
    "${SCRIPT_DIR}/vm-start-alpine.sh" --vm-name="${VM_NAME}"
fi

run_remote() {
    echo "$*" | limactl --tty=false shell "${VM_NAME}" -- sudo bash
}

assert_openrc_pid1() {
    if ! run_remote "rc-status -s >/dev/null 2>&1"; then
        die "OpenRC supervisor is not running (rc-status failed)"
    fi
}

log_step "T10-B+ Alpine openrc runtime smoke (${VM_NAME})"
assert_openrc_pid1

SSH_CONFIG="${HOME}/.lima/${VM_NAME}/ssh.config"
SSH_HOST="lima-${VM_NAME}"
STAGE="/tmp/edgelet-t10b-runtime"

if [[ "${AFTER_T10_B}" == true ]]; then
    log_info "Continuing after T10-B (no reinstall — avoids orphaned containerd-child)"
    if ! run_remote "test -x /usr/local/bin/edgelet"; then
        die "T10-B+ --after-t10-b requires edgelet installed from alpine-openrc-smoke.sh"
    fi
    log_info "Restart split OpenRC units after T10-B binary refresh"
    run_remote "rc-service edgelet-containerd restart"
    run_remote "rc-service edgelet restart"
else
    log_info "Fresh install path (stage + install.sh)"
    stage_install_bundle_ssh "${SSH_CONFIG}" "${SSH_HOST}" "${STAGE}" "${REPO_ROOT}" "${BIN}"
    run_remote "
        set -e
        STOP_OPENRC=0
        ${EMBED_RUNTIME_CLEANUP_SNIPPET}
        chmod +x ${STAGE}/install.sh ${STAGE}/edgelet ${STAGE}/scripts/edgelet-shutdown
        ${STAGE}/install.sh --bin-path=${STAGE}/edgelet --version=dev-t10b-runtime --arch=${TARGET_ARCH} --container-engine=edgelet
    "
fi

log_step "Fat runtime must be statically linked (Plan 10-8)"
run_remote "${EMBED_RUNTIME_ASSERT_STATIC_SNIPPET}"

log_step "Wait for EdgeletAPI + embedded engine (up to 240s)"
run_remote "API_WAIT_SEC=240
${EMBED_RUNTIME_WAIT_API_SNIPPET}"

log_step "EdgeletAPI and cgroupfs driver on non-systemd"
run_remote "
    set -e
    edgelet system status | grep -q iofogDaemon
    driver=\$(edgelet system status -o json | jq -r '.cgroupDriver')
    test \"\${driver}\" = cgroupfs
    grep -q '/run/edgelet/containerd.sock' /var/lib/edgelet-containerd/config.toml
    test \"\$(pgrep -f edgelet-containerd-child | wc -l | tr -d ' ')\" -eq 1
"

log_step "Deploy one busybox microservice"
run_remote "
    set -e
    edgelet registry ls
    cat >/tmp/t10b-runtime-ms.yaml <<'EOF'
apiVersion: edgelet.iofog.org/v1
kind: Microservice
metadata:
  name: t10b-runtime-ms
spec:
  image: docker.io/library/busybox:1.36
  registry: 1
  container:
    hostNetworkMode: false
    isPrivileged: false
    commands:
      - /bin/sh
      - -c
      - sleep 3600
  schedule: 50
EOF
    out=\$(edgelet deploy -f /tmp/t10b-runtime-ms.yaml 2>&1)
    echo \"\${out}\" | grep -q 'microservice manifest applied successfully'
    edgelet ms ls | grep -q t10b-runtime-ms
"

log_step "rc-service edgelet restart stability (bounded stop + runtime cleanup)"
run_remote "STOP_TIMEOUT_SEC=180
API_WAIT_SEC=180
${EMBED_RUNTIME_RESTART_SNIPPET}"
run_remote "
    set -e
    edgelet ms ls | grep -q t10b-runtime-ms
    test \"\$(pgrep -f edgelet-containerd-child | wc -l | tr -d ' ')\" -eq 1
    test -S /run/edgelet/containerd.sock
    test -S /run/edgelet/edgelet.sock
"

log_success "T10-B+ Alpine openrc runtime smoke passed"
