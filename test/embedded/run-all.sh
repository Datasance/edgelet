#!/usr/bin/env bash
# test/embedded/run-all.sh
#
# Master test runner for the edgelet embedded-containerd integration tests.
# Runs the full pipeline: setup → build → VM start → install → test → (stop).
#
# Usage:
#   ./test/embedded/run-all.sh [options]
#
# Options:
#   --skip-setup      Skip macOS prerequisite installation
#   --skip-build      Skip cross-compile (reuse existing binaries in build/)
#   --skip-start      Skip VM creation/start (VM must already be running)
#   --skip-stop       Keep VM running after tests (default: keep running)
#   --delete-vm       Stop AND delete the VM after tests
#   --vm-name=NAME    Lima VM name (default: iofog-test)
#   --arch=ARCH       Target Linux arch: arm64 | amd64 (default: auto-detect)
#   --timeout=N       Seconds to wait for VM readiness (default: 300)
#   --ci              Non-interactive CI mode (implies --delete-vm on failure)
#
# Exit code:
#   0  All tests passed
#   1  One or more tests failed (or setup/build/install error)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
source "${SCRIPT_DIR}/lib/log.sh"

###############################################################################
# Defaults
###############################################################################
SKIP_SETUP=false
SKIP_BUILD=false
SKIP_START=false
SKIP_STOP=true   # default: leave VM running for re-runs (faster iteration)
DELETE_VM=false
VM_NAME="iofog-test"
TIMEOUT=300
CI_MODE=false

HOST_ARCH="$(native_arch)"
[[ "${HOST_ARCH}" == "arm64" ]] && TARGET_ARCH="arm64" || TARGET_ARCH="amd64"

###############################################################################
# Argument parsing
###############################################################################
for arg in "$@"; do
    case "${arg}" in
        --skip-setup)   SKIP_SETUP=true ;;
        --skip-build)   SKIP_BUILD=true ;;
        --skip-start)   SKIP_START=true ;;
        --skip-stop)    SKIP_STOP=true ;;
        --delete-vm)    DELETE_VM=true; SKIP_STOP=false ;;
        --vm-name=*)    VM_NAME="${arg#*=}" ;;
        --arch=*)       TARGET_ARCH="${arg#*=}" ;;
        --timeout=*)    TIMEOUT="${arg#*=}" ;;
        --ci)           CI_MODE=true ;;
        --help|-h)
            sed -n '/^# Usage:/,/^[^#]/p' "$0" | grep '^#' | sed 's/^# \?//'
            exit 0
            ;;
        *) log_warn "Unknown option: ${arg}" ;;
    esac
done

###############################################################################
# Banner
###############################################################################
echo ""
echo -e "${_BOLD}======================================================================${_RESET}"
echo -e "${_BOLD}  Edgelet Embedded Containerd — Full Integration Test Suite${_RESET}"
echo -e "${_BOLD}  Host: $(uname -s)/${HOST_ARCH}   Target: linux/${TARGET_ARCH}   VM: ${VM_NAME}${_RESET}"
echo -e "${_BOLD}======================================================================${_RESET}"
echo ""

START_TIME=$(date +%s)
OVERALL_STATUS=0

###############################################################################
# Cleanup trap
###############################################################################
cleanup() {
    local exit_code=$?
    if [[ "${exit_code}" -ne 0 && "${CI_MODE}" == "true" && "${DELETE_VM}" == "true" ]]; then
        log_warn "CI mode: cleaning up VM after failure"
        "${SCRIPT_DIR}/vm-stop.sh" --vm-name="${VM_NAME}" --delete 2>/dev/null || true
    fi
}
trap cleanup EXIT

###############################################################################
# Step 1: Prerequisites
###############################################################################
if [[ "${SKIP_SETUP}" == "false" ]]; then
    log_step "Step 1/5: macOS prerequisites"
    "${SCRIPT_DIR}/setup.sh"
else
    log_info "Skipping setup (--skip-setup)"
fi

###############################################################################
# Step 2: Build binaries
###############################################################################
if [[ "${SKIP_BUILD}" == "false" ]]; then
    log_step "Step 2/5: Building linux/${TARGET_ARCH} binaries"
    cd "${REPO_ROOT}"
    "${SCRIPT_DIR}/build.sh" --arch="${TARGET_ARCH}"
else
    log_info "Skipping build (--skip-build)"
    EDGELET_BIN="${REPO_ROOT}/build/edgelet-linux-${TARGET_ARCH}"
    [[ -f "${EDGELET_BIN}" ]] || \
        die "No pre-built binary at ${EDGELET_BIN}. Remove --skip-build to build."
fi

###############################################################################
# Step 3: Start VM
###############################################################################
if [[ "${SKIP_START}" == "false" ]]; then
    log_step "Step 3/5: Starting Lima VM '${VM_NAME}'"
    "${SCRIPT_DIR}/vm-start.sh" --vm-name="${VM_NAME}" --timeout="${TIMEOUT}"
else
    log_info "Skipping VM start (--skip-start)"
fi

###############################################################################
# Step 4: Install binaries in VM
###############################################################################
log_step "Step 4/5: Installing agent in VM"
"${SCRIPT_DIR}/vm-install.sh" --vm-name="${VM_NAME}" --arch="${TARGET_ARCH}"

###############################################################################
# Step 5: Run tests
###############################################################################
log_step "Step 5/5: Running integration tests"
set +e
"${SCRIPT_DIR}/vm-test.sh" --vm-name="${VM_NAME}"
TEST_EXIT=$?
set -e

if [[ "${TEST_EXIT}" -ne 0 ]]; then
    OVERALL_STATUS=1
fi

###############################################################################
# Stop / delete VM (optional)
###############################################################################
if [[ "${DELETE_VM}" == "true" ]]; then
    log_step "Tearing down VM '${VM_NAME}'"
    "${SCRIPT_DIR}/vm-stop.sh" --vm-name="${VM_NAME}" --delete
elif [[ "${SKIP_STOP}" == "false" ]]; then
    "${SCRIPT_DIR}/vm-stop.sh" --vm-name="${VM_NAME}"
else
    echo ""
    log_info "VM '${VM_NAME}' is still running."
    log_info "  Re-run tests only: ./test/embedded/vm-test.sh --vm-name=${VM_NAME}"
    log_info "  Open shell:        limactl shell ${VM_NAME}"
    log_info "  Stop VM:           limactl stop ${VM_NAME}"
    log_info "  Delete VM:         limactl delete ${VM_NAME}"
fi

###############################################################################
# Final summary
###############################################################################
END_TIME=$(date +%s)
ELAPSED=$(( END_TIME - START_TIME ))

echo ""
echo -e "${_BOLD}======================================================================"
if [[ "${OVERALL_STATUS}" -eq 0 ]]; then
    echo -e "${_GREEN}  ALL TESTS PASSED${_RESET}${_BOLD}  (${ELAPSED}s)"
else
    echo -e "${_RED}  TESTS FAILED${_RESET}${_BOLD}  (${ELAPSED}s)"
fi
echo -e "${_BOLD}======================================================================${_RESET}"
echo ""

exit "${OVERALL_STATUS}"
