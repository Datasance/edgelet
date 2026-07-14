#!/usr/bin/env bash
# test/init/vm-start-alpine.sh
#
# Creates (if needed) and starts the edgelet-openrc Lima VM (Alpine + OpenRC).
# Waits until SSH and provisioning are ready.
#
# Usage:
#   ./test/init/vm-start-alpine.sh [--vm-name=edgelet-openrc] [--timeout=300]

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
# shellcheck source=test/embedded/lib/log.sh
source "${REPO_ROOT}/test/embedded/lib/log.sh"
# shellcheck source=test/init/lib/lima.sh
source "${SCRIPT_DIR}/lib/lima.sh"

VM_NAME="edgelet-openrc"
TIMEOUT=300

for arg in "$@"; do
    case "${arg}" in
        --vm-name=*) VM_NAME="${arg#*=}" ;;
        --timeout=*) TIMEOUT="${arg#*=}" ;;
        -h|--help)
            echo "Usage: $0 [--vm-name=NAME] [--timeout=SECS]"
            exit 0
            ;;
    esac
done

LIMA_YAML="${SCRIPT_DIR}/lima-alpine-openrc.yaml"

if ! command -v limactl &>/dev/null; then
    die "limactl not found. Install Lima: https://lima-vm.io/"
fi
log_info "limactl: $(limactl --version)"

VM_STATUS="$(lima_vm_status "${VM_NAME}")"
if [[ -n "${VM_STATUS}" ]]; then
    log_info "VM '${VM_NAME}' already exists (status: ${VM_STATUS})"
else
    log_step "Creating Lima VM '${VM_NAME}' from ${LIMA_YAML}"
    limactl --tty=false create --name="${VM_NAME}" "${LIMA_YAML}"
    log_ok "VM '${VM_NAME}' created"
fi

VM_STATUS="$(lima_vm_status "${VM_NAME}")"
if [[ "${VM_STATUS}" == "Running" ]]; then
    log_info "VM '${VM_NAME}' is already running"
else
    log_step "Starting VM '${VM_NAME}'..."
    limactl --tty=false start --timeout=1200s "${VM_NAME}"
fi

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

log_step "Ensuring Alpine openrc smoke guest packages (jq, bash, curl)..."
limactl --tty=false shell "${VM_NAME}" -- sudo sh -c '
    set -e
    command -v jq >/dev/null 2>&1 || apk add --no-cache jq bash curl ca-certificates iproute2 file binutils
    mkdir -p /var/log/edgelet /etc/edgelet /var/lib/edgelet /run/edgelet /run/containerd /etc/cni/net.d
'

INIT_LINK="$(limactl --tty=false shell "${VM_NAME}" -- readlink -f /sbin/init 2>/dev/null || true)"
log_info "PID 1 binary: ${INIT_LINK:-unknown} (Alpine often links to busybox; OpenRC runs via inittab)"

if ! limactl --tty=false shell "${VM_NAME}" -- sudo rc-status -s >/dev/null 2>&1; then
    die "OpenRC supervisor is not running (rc-status failed).
Use ./test/init/vm-stop-alpine.sh --delete then ./test/init/vm-start-alpine.sh
Do not use limactl create template:alpine — that image does not run OpenRC."
fi
log_ok "OpenRC supervisor active"

# log_step "Ensuring bpf filesystem for crun (embedded runtime on minimal hosts)..."
# limactl --tty=false shell "${VM_NAME}" -- sudo sh -c '
#     mountpoint -q /sys/fs/bpf || mount -t bpf bpf /sys/fs/bpf 2>/dev/null || true
#     mkdir -p /sys/fs/bpf/crun/k8s_io
#     chmod 755 /sys/fs/bpf/crun /sys/fs/bpf/crun/k8s_io 2>/dev/null || true
# '

log_step "Ensuring bpf filesystem for crun (embedded runtime on minimal hosts)..."
limactl --tty=false shell "${VM_NAME}" -- sudo sh -c '
    mountpoint -q /sys/fs/bpf || mount -t bpf bpf /sys/fs/bpf 2>/dev/null || true
'

log_success "VM '${VM_NAME}' is ready for Alpine openrc smoke / Alpine openrc runtime smoke (./test/init/alpine-openrc-smoke.sh)"
