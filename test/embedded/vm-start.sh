#!/usr/bin/env bash
# test/embedded/vm-start.sh
#
# Creates (if needed) and starts the iofog-test Lima VM.
# Waits until the VM is reachable via SSH before returning.
#
# Usage:
#   ./test/embedded/vm-start.sh [--vm-name=iofog-test] [--timeout=300] [--lima-yaml=PATH]

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/lib/log.sh"

VM_NAME="iofog-test"
TIMEOUT=300  # seconds to wait for VM readiness
LIMA_YAML="${SCRIPT_DIR}/lima-ubuntu.yaml"

for arg in "$@"; do
    case "${arg}" in
        --vm-name=*) VM_NAME="${arg#*=}" ;;
        --timeout=*) TIMEOUT="${arg#*=}" ;;
        --lima-yaml=*) LIMA_YAML="${arg#*=}" ;;
    esac
done

###############################################################################
# Check limactl
###############################################################################
if ! command -v limactl &>/dev/null; then
    die "limactl not found. Run ./test/embedded/setup.sh first."
fi
log_info "limactl: $(limactl --version)"

###############################################################################
# Create VM if it doesn't exist
# Lima 2.x outputs JSONL (one JSON object per line), not a JSON array.
###############################################################################
vm_status() {
    # Works with both Lima 1.x (array) and 2.x (JSONL) output formats.
    limactl list --json 2>/dev/null \
        | jq -r "select(.name == \"${VM_NAME}\") | .status" 2>/dev/null \
        | head -1 || true
}

VM_STATUS="$(vm_status)"
if [[ -n "${VM_STATUS}" ]]; then
    log_info "VM '${VM_NAME}' already exists (status: ${VM_STATUS})"
else
    log_step "Creating Lima VM '${VM_NAME}' from ${LIMA_YAML}"
    limactl create --name="${VM_NAME}" "${LIMA_YAML}"
    log_ok "VM '${VM_NAME}' created"
fi

###############################################################################
# Start VM if not running
###############################################################################
VM_STATUS="$(vm_status)"
if [[ "${VM_STATUS}" == "Running" ]]; then
    log_info "VM '${VM_NAME}' is already running"
else
    log_step "Starting VM '${VM_NAME}'..."
    if ! limactl start --timeout=1200s "${VM_NAME}"; then
        # lima-ubuntu-v1.yaml reboots once for GRUB hybrid cgroup; first start may fail the probe.
        log_warn "lima start failed — retrying (common after v1 GRUB reboot)..."
        sleep 15
        limactl start --timeout=1200s "${VM_NAME}" \
            || die "Could not start VM '${VM_NAME}' (see ~/.lima/${VM_NAME}/ha.stderr.log)"
    fi
fi

###############################################################################
# Wait for SSH readiness
###############################################################################
log_step "Waiting for VM to be SSH-ready (timeout: ${TIMEOUT}s)"

elapsed=0
interval=5
until limactl --tty=false shell "${VM_NAME}" -- echo "SSH OK" &>/dev/null; do
    if (( elapsed >= TIMEOUT )); then
        die "VM '${VM_NAME}' did not become SSH-ready within ${TIMEOUT}s"
    fi
    echo -n "."
    sleep "${interval}"
    (( elapsed += interval )) || true
done
echo ""

###############################################################################
# Wait for provision script to finish (check for marker)
###############################################################################
log_info "Waiting for VM provisioning to complete..."
elapsed=0
until limactl --tty=false shell "${VM_NAME}" -- bash -c "command -v curl &>/dev/null" 2>/dev/null; do
    if (( elapsed >= TIMEOUT )); then
        die "VM provisioning did not complete within ${TIMEOUT}s"
    fi
    echo -n "."
    sleep "${interval}"
    (( elapsed += interval )) || true
done
echo ""

###############################################################################
# Verify kernel modules
###############################################################################
log_step "Verifying kernel capabilities"
limactl --tty=false shell "${VM_NAME}" -- bash -c "modprobe overlay 2>/dev/null || true"
limactl --tty=false shell "${VM_NAME}" -- bash -c "modprobe br_netfilter 2>/dev/null || true"

OVERLAY_OK=$(limactl --tty=false shell "${VM_NAME}" -- bash -c \
    "grep -q overlay /proc/filesystems && echo yes || echo no" 2>/dev/null)
if [[ "${OVERLAY_OK}" == "yes" ]]; then
    log_ok "overlayfs available"
else
    log_warn "overlayfs not available — containerd snapshotter may need 'native' fallback"
fi

CGROUP_VER=$(limactl --tty=false shell "${VM_NAME}" -- bash -c \
    "stat -fc %T /sys/fs/cgroup 2>/dev/null || echo unknown")
log_info "cgroup filesystem type: ${CGROUP_VER}"
if [[ "${CGROUP_VER}" == "cgroup2fs" ]]; then
    log_ok "cgroups v2 (unified hierarchy)"
else
    log_warn "cgroups v1 detected — edgelet will still work but cgroups v2 is preferred"
fi

###############################################################################
# Done
###############################################################################
log_success "VM '${VM_NAME}' is ready"
