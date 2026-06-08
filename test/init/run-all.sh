#!/usr/bin/env bash
# test/init/run-all.sh — init IT master runner.
#
# Usage:
#   ./test/init/run-all.sh
#   ./test/init/run-all.sh --case=systemd
#   ./test/init/run-all.sh --case=openrc   # auto-starts edgelet-openrc via vm-start-alpine.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
# shellcheck source=test/embedded/lib/log.sh
source "${REPO_ROOT}/test/embedded/lib/log.sh"

CASE="all"
SKIP_CGROUP_REGRESSION=false

for arg in "$@"; do
    case "${arg}" in
        --case=*) CASE="${arg#*=}" ;;
        --skip-cgroup-regression) SKIP_CGROUP_REGRESSION=true ;;
        -h|--help)
            echo "Usage: $0 [--case=all|systemd|openrc|cgroup] [--skip-cgroup-regression]"
            exit 0
            ;;
    esac
done

run_systemd() {
    log_step "systemd install smoke systemd install smoke"
    "${SCRIPT_DIR}/systemd-install-smoke.sh"
}

run_openrc() {
    log_step "Alpine openrc smoke Alpine openrc smoke"
    if ! command -v limactl >/dev/null; then
        log_warn "limactl not found — skipping Alpine openrc smoke"
        return 0
    fi
    "${SCRIPT_DIR}/vm-start-alpine.sh"
    "${SCRIPT_DIR}/alpine-openrc-smoke.sh"
    "${SCRIPT_DIR}/alpine-openrc-runtime-smoke.sh" --after-t10-b
}

run_cgroup_regression() {
    log_step "cgroup unit regression (embedded engine)"
    (cd "${REPO_ROOT}" && go test ./internal/cgroups/...)
}

FAILED=0

case "${CASE}" in
    all)
        run_cgroup_regression || FAILED=1
        run_systemd || FAILED=1
        run_openrc || FAILED=1
        ;;
    systemd)
        run_systemd || FAILED=1
        ;;
    openrc)
        run_openrc || FAILED=1
        ;;
    cgroup)
        run_cgroup_regression || FAILED=1
        ;;
    *)
        die "Unknown --case=${CASE}"
        ;;
esac

if [[ "${FAILED}" -ne 0 ]]; then
    die "test/init/run-all.sh: one or more gates failed"
fi

log_success "test/init/run-all.sh passed (case=${CASE})"
