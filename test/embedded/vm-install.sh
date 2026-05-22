#!/usr/bin/env bash
# test/embedded/vm-install.sh
#
# Copies the pre-built iofog-agent binaries into the Lima VM, writes the
# default config, and starts iofog-agentd as a systemd service.
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

DAEMON_BIN="${REPO_ROOT}/build/iofog-agentd-linux-${TARGET_ARCH}-full"
CLI_BIN="${REPO_ROOT}/build/iofog-agent-linux-${TARGET_ARCH}-full"
CONFIG_SRC="${REPO_ROOT}/packaging/iofog-agent/etc/iofog-agent/config_new.yaml"
CERT_SRC="${REPO_ROOT}/packaging/iofog-agent/etc/iofog-agent/cert_new.crt"
SETUP_SCRIPT="${SCRIPT_DIR}/vm-setup-inside.sh"

###############################################################################
# Validate files exist on the host
###############################################################################
[[ -f "${DAEMON_BIN}" ]]    || die "Daemon binary not found: ${DAEMON_BIN}\nRun ./test/embedded/build.sh first."
[[ -f "${CLI_BIN}" ]]       || die "CLI binary not found: ${CLI_BIN}\nRun ./test/embedded/build.sh first."
[[ -f "${CONFIG_SRC}" ]]    || die "Config template not found: ${CONFIG_SRC}"
[[ -f "${CERT_SRC}" ]]      || die "Cert template not found: ${CERT_SRC}"
[[ -f "${SETUP_SCRIPT}" ]]  || die "Setup script not found: ${SETUP_SCRIPT}"

log_step "Installing iofog-agent binaries into VM '${VM_NAME}'"
log_info "Daemon:  ${DAEMON_BIN}"
log_info "CLI:     ${CLI_BIN}"
log_info "Config:  ${CONFIG_SRC}"
log_info "Cert:    ${CERT_SRC}"
###############################################################################
# Copy binaries via scp (bypasses Lima's 9p mount caching).
# Lima SSH config lives at ~/.lima/<vm>/ssh.config.
###############################################################################
SSH_CONFIG="${HOME}/.lima/${VM_NAME}/ssh.config"
SSH_HOST="lima-${VM_NAME}"
# Staging dir inside the VM (writable by SSH user, accessible by root via sudo)
VM_STAGE="/tmp/iofog-install"

log_info "Copying binaries via SCP (bypasses Lima mount cache)..."
# Create staging dir in VM
ssh -F "${SSH_CONFIG}" "${SSH_HOST}" "mkdir -p ${VM_STAGE}"
# Copy files
scp -F "${SSH_CONFIG}" -q "${DAEMON_BIN}" "${SSH_HOST}:${VM_STAGE}/iofog-agentd"
scp -F "${SSH_CONFIG}" -q "${CLI_BIN}"    "${SSH_HOST}:${VM_STAGE}/iofog-agent"
scp -F "${SSH_CONFIG}" -q "${CONFIG_SRC}" "${SSH_HOST}:${VM_STAGE}/config_new.yaml"
scp -F "${SSH_CONFIG}" -q "${CERT_SRC}"   "${SSH_HOST}:${VM_STAGE}/cert_new.crt"
log_info "SCP transfer complete."

###############################################################################
# Run the setup script via stdin piping (avoids 9p caching for scripts too).
# Pass the staging paths as positional arguments to vm-setup-inside.sh.
###############################################################################
log_info "Running VM setup script (this may take 60-90s on first start)..."

cat "${SETUP_SCRIPT}" | \
    limactl --tty=false shell "${VM_NAME}" -- \
    sudo bash -s -- \
        "${VM_STAGE}/iofog-agentd" \
        "${VM_STAGE}/iofog-agent" \
        "${VM_STAGE}/config_new.yaml" \
        "${VM_STAGE}/cert_new.crt"

###############################################################################
# Done
###############################################################################
log_success "Installation complete in VM '${VM_NAME}'"
echo ""
echo "  Daemon logs:  limactl shell ${VM_NAME}  (then: sudo journalctl -fu iofog-agentd)"
echo "  CLI status:   limactl shell ${VM_NAME}  (then: sudo iofog-agent system status)"
