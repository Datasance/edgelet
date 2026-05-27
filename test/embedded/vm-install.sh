#!/usr/bin/env bash
# test/embedded/vm-install.sh
#
# Copies the pre-built edgelet binary into the Lima VM, writes the default
# config, and starts edgelet as a systemd service.
#
# Binaries are transferred via scp (using Lima's SSH config) to bypass
# Lima's 9p filesystem mount caching, which can serve stale file content.
# Setup commands are piped via stdin for the same reason.
#
# Usage:
#   ./test/embedded/vm-install.sh [--vm-name=iofog-test] [--arch=arm64]

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
source "${SCRIPT_DIR}/lib/log.sh"

VM_NAME="iofog-test"
HOST_ARCH="$(native_arch)"
[[ "${HOST_ARCH}" == "arm64" ]] && TARGET_ARCH="arm64" || TARGET_ARCH="amd64"

for arg in "$@"; do
    case "${arg}" in
        --vm-name=*) VM_NAME="${arg#*=}" ;;
        --arch=*)    TARGET_ARCH="${arg#*=}" ;;
    esac
done

EDGELET_BIN="${REPO_ROOT}/build/edgelet-linux-${TARGET_ARCH}"
CONFIG_SRC="${REPO_ROOT}/packaging/edgelet/etc/edgelet/config_new.yaml"
CERT_SRC="${REPO_ROOT}/packaging/edgelet/etc/edgelet/cert_new.crt"
SETUP_SCRIPT="${SCRIPT_DIR}/vm-setup-inside.sh"

###############################################################################
# Validate files exist on the host
###############################################################################
[[ -f "${EDGELET_BIN}" ]]   || die "Edgelet binary not found: ${EDGELET_BIN}\nRun ./test/embedded/build.sh first."
[[ -f "${CONFIG_SRC}" ]]    || die "Config template not found: ${CONFIG_SRC}"
[[ -f "${CERT_SRC}" ]]      || die "Cert template not found: ${CERT_SRC}"
[[ -f "${SETUP_SCRIPT}" ]]  || die "Setup script not found: ${SETUP_SCRIPT}"

log_step "Installing edgelet into VM '${VM_NAME}'"
log_info "Binary:  ${EDGELET_BIN}"
log_info "Config:  ${CONFIG_SRC}"
log_info "Cert:    ${CERT_SRC}"
###############################################################################
# Copy binary via scp (bypasses Lima's 9p mount caching).
# Lima SSH config lives at ~/.lima/<vm>/ssh.config.
###############################################################################
SSH_CONFIG="${HOME}/.lima/${VM_NAME}/ssh.config"
SSH_HOST="lima-${VM_NAME}"
VM_STAGE="/tmp/edgelet-install"

log_info "Copying files via SCP (bypasses Lima mount cache)..."
ssh -F "${SSH_CONFIG}" "${SSH_HOST}" "mkdir -p ${VM_STAGE}"
scp -F "${SSH_CONFIG}" -q "${EDGELET_BIN}" "${SSH_HOST}:${VM_STAGE}/edgelet"
scp -F "${SSH_CONFIG}" -q "${CONFIG_SRC}" "${SSH_HOST}:${VM_STAGE}/config_new.yaml"
scp -F "${SSH_CONFIG}" -q "${CERT_SRC}"   "${SSH_HOST}:${VM_STAGE}/cert_new.crt"
log_info "SCP transfer complete."

###############################################################################
# Run the setup script via stdin piping (avoids 9p caching for scripts too).
###############################################################################
log_info "Running VM setup script (this may take 60-90s on first start)..."

cat "${SETUP_SCRIPT}" | \
    limactl --tty=false shell "${VM_NAME}" -- \
    sudo bash -s -- \
        "${VM_STAGE}/edgelet" \
        "${VM_STAGE}/config_new.yaml" \
        "${VM_STAGE}/cert_new.crt"

###############################################################################
# Done
###############################################################################
log_success "Installation complete in VM '${VM_NAME}'"
echo ""
echo "  Daemon logs:  limactl shell ${VM_NAME}  (then: sudo journalctl -fu edgelet)"
echo "  CLI status:   limactl shell ${VM_NAME}  (then: sudo edgelet system status)"
