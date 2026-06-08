#!/usr/bin/env bash
# Workload continuity IT master runner.
#
# Usage:
#   ./test/workload-continuity/run-all.sh
#   ./test/workload-continuity/run-all.sh --case=docker-restart
#   ./test/workload-continuity/run-all.sh --case=embedded-restart
#   ./test/workload-continuity/run-all.sh --skip-build
#   ./test/workload-continuity/run-all.sh --skip-setup

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
# shellcheck source=test/embedded/lib/log.sh
source "${REPO_ROOT}/test/embedded/lib/log.sh"
# shellcheck source=test/workload-continuity/lib/ensure-vm.sh
source "${SCRIPT_DIR}/lib/ensure-vm.sh"

CASE=""
SKIP_BUILD=false
SKIP_SETUP=false
VM_DOCKER="edgelet-engine-lifecycle"
VM_EMBED="iofog-test"
TARGET_ARCH="$(wc_target_arch)"
MS_FIXTURE="${SCRIPT_DIR}/fixtures/workload-ms.yaml"

for arg in "$@"; do
    case "${arg}" in
        --case=*) CASE="${arg#*=}" ;;
        --skip-build) SKIP_BUILD=true ;;
        --skip-setup) SKIP_SETUP=true ;;
        --vm-docker=*) VM_DOCKER="${arg#*=}" ;;
        --vm-embed=*) VM_EMBED="${arg#*=}" ;;
        -h|--help)
            cat <<EOF
Usage: $0 [--case=NAME] [--skip-build] [--skip-setup] [--vm-docker=NAME] [--vm-embed=NAME]

Cases:
  docker-restart              docker-restart only
  embedded-restart            embedded control restart only
  embedded-runtime-restart    embedded data-plane restart only
  (default)                   docker-restart, engine-lifecycle regression, embedded control restart, embedded data-plane restart when prerequisites allow
EOF
            exit 0
            ;;
    esac
done

[[ -f "${MS_FIXTURE}" ]] || die "Missing fixture ${MS_FIXTURE}"

needs_build() {
    case "${CASE}" in
        docker-restart|embedded-restart|embedded-runtime-restart|"") return 0 ;;
        *) return 1 ;;
    esac
}

if [[ "${SKIP_BUILD}" == false ]] && needs_build; then
    log_step "Building edgelet once (test/embedded/build.sh)"
    "${REPO_ROOT}/test/embedded/build.sh" --arch="${TARGET_ARCH}"
fi

FAIL=0

run_docker_restart() {
    ensure_docker_vm "${REPO_ROOT}" "${VM_DOCKER}" "${SKIP_SETUP}" "${MS_FIXTURE}"
    "${SCRIPT_DIR}/docker-restart.sh" --vm-name="${VM_DOCKER}" || return 1
}

run_engine_lifecycle_regression() {
    log_info "engine-lifecycle regression regression: engine-lifecycle"
    # VM already started by ensure_docker_vm; only run switch tests.
    "${REPO_ROOT}/test/engine-lifecycle/run-all.sh" \
        --skip-build --skip-setup --skip-start --vm-name="${VM_DOCKER}" || return 1
}

run_embedded_cases() {
    if ensure_embedded_split "${REPO_ROOT}" "${VM_EMBED}" "${TARGET_ARCH}" "${MS_FIXTURE}" true; then
        "${SCRIPT_DIR}/embedded-restart.sh" --vm-name="${VM_EMBED}" || return 1
        "${SCRIPT_DIR}/embedded-runtime-restart.sh" --vm-name="${VM_EMBED}" || return 1
    else
        return 1
    fi
}

case "${CASE}" in
    docker-restart)
        run_docker_restart || FAIL=1
        ;;
    embedded-restart)
        ensure_embedded_split "${REPO_ROOT}" "${VM_EMBED}" "${TARGET_ARCH}" "${MS_FIXTURE}" true \
            || FAIL=1
        [[ "${FAIL}" -eq 0 ]] && "${SCRIPT_DIR}/embedded-restart.sh" --vm-name="${VM_EMBED}" || FAIL=1
        ;;
    embedded-runtime-restart)
        ensure_embedded_split "${REPO_ROOT}" "${VM_EMBED}" "${TARGET_ARCH}" "${MS_FIXTURE}" true \
            || FAIL=1
        [[ "${FAIL}" -eq 0 ]] && "${SCRIPT_DIR}/embedded-runtime-restart.sh" --vm-name="${VM_EMBED}" || FAIL=1
        ;;
    "")
        run_docker_restart || FAIL=1
        [[ "${FAIL}" -eq 0 ]] && run_engine_lifecycle_regression || FAIL=1
        [[ "${FAIL}" -eq 0 ]] && run_embedded_cases || FAIL=1
        ;;
    *)
        die "Unknown --case=${CASE}"
        ;;
esac

if [[ "${FAIL}" -ne 0 ]]; then
    die "workload-continuity run-all failed"
fi
log_success "workload-continuity run-all complete"
