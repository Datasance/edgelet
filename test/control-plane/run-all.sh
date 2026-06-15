#!/usr/bin/env bash
# ControlPlane integration tests — embedded CP apply through CP DNS resolution + init and workload continuity regression.
#
# Usage:
#   ./test/control-plane/run-all.sh
#   ./test/control-plane/run-all.sh --case=embedded
#   ./test/control-plane/run-all.sh --case=docker
#   ./test/control-plane/run-all.sh --skip-build --skip-regression
#
# Requires: limactl, Lima VMs (iofog-test, edgelet-engine-lifecycle), network for controller image pull.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
# shellcheck source=test/lima/lib/log.sh
source "${REPO_ROOT}/test/lima/lib/log.sh"
# shellcheck source=test/lima/lib/arch.sh
source "${REPO_ROOT}/test/lima/lib/arch.sh"

CASE=""
SKIP_BUILD=false
SKIP_SETUP=false
SKIP_REGRESSION=false
VM_EMBED="iofog-test"
VM_DOCKER="edgelet-engine-lifecycle"
TARGET_ARCH="$(lima_target_arch)"
FIXTURE="${SCRIPT_DIR}/fixtures/controlplane-it.yaml"

for arg in "$@"; do
    case "${arg}" in
        --case=*) CASE="${arg#*=}" ;;
        --skip-build) SKIP_BUILD=true ;;
        --skip-setup) SKIP_SETUP=true ;;
        --skip-regression) SKIP_REGRESSION=true ;;
        --vm-embed=*) VM_EMBED="${arg#*=}" ;;
        --vm-docker=*) VM_DOCKER="${arg#*=}" ;;
        --fixture=*) FIXTURE="${arg#*=}" ;;
        -h|--help)
            cat <<EOF
Usage: $0 [options]

Runs ControlPlane IT:
  embedded CP apply  embedded (iofog-test)
  docker CP apply  docker (edgelet-engine-lifecycle)
  controller status API  /api/v3/status (in t12-embedded.sh / t12-docker.sh)
  CP lifecycle (unprovisioned)  ms rm allowed + reconcile; controlplane delete
  CP DNS resolution  DNS 3 FQDNs from fixture metadata (embedded only)

Options:
  --case=embedded|docker|all   default: all
  --skip-build                 reuse build/edgelet-linux-<arch>
  --skip-setup                 skip macOS prerequisite setup
  --skip-regression            skip workload-continuity + embedded run-all
  --vm-embed=NAME              default: iofog-test
  --vm-docker=NAME             default: edgelet-engine-lifecycle
  --fixture=PATH               default: test/control-plane/fixtures/controlplane-it.yaml
EOF
            exit 0
            ;;
    esac
done

[[ -f "${FIXTURE}" ]] || die "Missing IT fixture ${FIXTURE}"

command -v limactl >/dev/null || die "limactl required for control-plane IT"

if [[ "${SKIP_BUILD}" == false ]]; then
    log_step "Building edgelet linux/${TARGET_ARCH}"
    "${REPO_ROOT}/test/embedded/build.sh" --arch="${TARGET_ARCH}"
fi

FAIL=0
_SETUP_FLAG=()
[[ "${SKIP_SETUP}" == true ]] && _SETUP_FLAG=(--skip-setup)
_FIXTURE_FLAG=(--fixture="${FIXTURE}")

run_embedded() {
    chmod +x "${SCRIPT_DIR}/t12-embedded.sh"
    "${SCRIPT_DIR}/t12-embedded.sh" \
        --vm-name="${VM_EMBED}" "${_SETUP_FLAG[@]}" "${_FIXTURE_FLAG[@]}" || return 1
}

run_docker() {
    chmod +x "${SCRIPT_DIR}/t12-docker.sh"
    "${SCRIPT_DIR}/t12-docker.sh" \
        --vm-name="${VM_DOCKER}" "${_SETUP_FLAG[@]}" "${_FIXTURE_FLAG[@]}" || return 1
}

run_regression() {
    log_step "Regression: workload-continuity"
    "${REPO_ROOT}/test/workload-continuity/run-all.sh" \
        --skip-build --skip-setup \
        --vm-docker="${VM_DOCKER}" --vm-embed="${VM_EMBED}" || return 1
    log_step "Regression: embedded vm-test (no reinstall)"
    chmod +x "${REPO_ROOT}/test/embedded/vm-test.sh"
    "${REPO_ROOT}/test/embedded/vm-test.sh" --vm-name="${VM_EMBED}" || return 1
}

case "${CASE}" in
    embedded) run_embedded || FAIL=1 ;;
    docker)   run_docker || FAIL=1 ;;
    all|"")   run_embedded || FAIL=1
              [[ "${FAIL}" -eq 0 ]] && run_docker || FAIL=1 ;;
    *)
        die "Unknown --case=${CASE} (embedded|docker|all)"
        ;;
esac

if [[ "${FAIL}" -eq 0 && "${SKIP_REGRESSION}" == false && ( "${CASE}" == "" || "${CASE}" == "all" ) ]]; then
    run_regression || FAIL=1
fi

if [[ "${FAIL}" -ne 0 ]]; then
    die "control-plane run-all failed"
fi

if [[ "${SKIP_REGRESSION}" == true ]]; then
    log_success "control-plane run-all complete (regression skipped)"
else
    log_success "control-plane run-all complete (+ regression)"
fi
