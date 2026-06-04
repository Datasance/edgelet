#!/usr/bin/env bash
# test/embedded/vm-install.sh
#
# install via production install.sh + runtime split units
# (edgelet-containerd.service + edgelet.service). No host-only bpf/crun prep.
#
# Usage:
#   ./test/embedded/vm-install.sh [--vm-name=iofog-test] [--arch=arm64]

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
# shellcheck source=test/lima/lib/log.sh
source "${REPO_ROOT}/test/lima/lib/log.sh"
# shellcheck source=test/lima/lib/arch.sh
source "${REPO_ROOT}/test/lima/lib/arch.sh"
# shellcheck source=test/lima/lib/remote.sh
source "${REPO_ROOT}/test/lima/lib/remote.sh"
# shellcheck source=test/lima/lib/split-gate.sh
source "${REPO_ROOT}/test/lima/lib/split-gate.sh"
# shellcheck source=test/lima/lib/install-split.sh
source "${REPO_ROOT}/test/lima/lib/install-split.sh"

VM_NAME="iofog-test"
TARGET_ARCH="$(lima_target_arch)"

for arg in "$@"; do
    case "${arg}" in
        --vm-name=*) VM_NAME="${arg#*=}" ;;
        --arch=*)    TARGET_ARCH="${arg#*=}" ;;
    esac
done

if lima_embedded_split_active "${VM_NAME}" 2>/dev/null; then
    if lima_wait_edgelet_api "${VM_NAME}" 180 2>/dev/null; then
        log_info "Split install already active on '${VM_NAME}' — skipping reinstall"
        exit 0
    fi
    log_info "Split units active but runtime not ready — reinstalling"
fi

lima_install_embedded_split "${REPO_ROOT}" "${VM_NAME}" "${TARGET_ARCH}" "dev-embedded"

log_success "Installation complete in VM '${VM_NAME}'"
echo ""
echo "  Daemon logs:  limactl shell ${VM_NAME}  (then: sudo journalctl -fu edgelet)"
echo "  Runtime logs: limactl shell ${VM_NAME}  (then: sudo journalctl -fu edgelet-containerd)"
echo "  CLI status:   limactl shell ${VM_NAME}  (then: sudo edgelet system status)"
