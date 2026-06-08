#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
source "${SCRIPT_DIR}/lib/log.sh"

VM_NAME="edgelet-engine-lifecycle"
START_ENGINE="edgelet"
HOST_ARCH="$(native_arch)"
[[ "${HOST_ARCH}" == "arm64" ]] && TARGET_ARCH="arm64" || TARGET_ARCH="amd64"

for arg in "$@"; do
    case "${arg}" in
        --vm-name=*) VM_NAME="${arg#*=}" ;;
        --start-engine=*) START_ENGINE="${arg#*=}" ;;
        --arch=*) TARGET_ARCH="${arg#*=}" ;;
    esac
done

EDGELET_BIN="${REPO_ROOT}/build/edgelet-linux-${TARGET_ARCH}"
CONFIG_SRC="${REPO_ROOT}/packaging/edgelet/etc/edgelet/config_new.yaml"
CERT_SRC="${REPO_ROOT}/packaging/edgelet/etc/edgelet/controller-ca.sample.crt"
SETUP_SCRIPT="${SCRIPT_DIR}/vm-setup-inside.sh"

[[ -f "${EDGELET_BIN}" ]] || die "Missing ${EDGELET_BIN}"
SSH_CONFIG="${HOME}/.lima/${VM_NAME}/ssh.config"
SSH_HOST="lima-${VM_NAME}"
VM_STAGE="/tmp/edgelet-engine-lifecycle-install"

ssh -F "${SSH_CONFIG}" "${SSH_HOST}" "mkdir -p ${VM_STAGE}"
scp -F "${SSH_CONFIG}" -q "${EDGELET_BIN}" "${SSH_HOST}:${VM_STAGE}/edgelet"
scp -F "${SSH_CONFIG}" -q "${CONFIG_SRC}" "${SSH_HOST}:${VM_STAGE}/config_new.yaml"
scp -F "${SSH_CONFIG}" -q "${CERT_SRC}" "${SSH_HOST}:${VM_STAGE}/cert_new.crt"

cat "${SETUP_SCRIPT}" | limactl --tty=false shell "${VM_NAME}" -- \
    sudo bash -s -- \
        "${VM_STAGE}/edgelet" \
        "${VM_STAGE}/config_new.yaml" \
        "${VM_STAGE}/cert_new.crt" \
        "${START_ENGINE}"

log_success "Installed edgelet with start engine ${START_ENGINE}"
