#!/usr/bin/env bash
# test/workload-continuity/lib/ensure-vm.sh
# VM prep for Plan 11 workload-continuity IT. Source from run-all.sh — do not execute directly.

# shellcheck shell=bash

REPO_ROOT_WC="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
# shellcheck source=test/lima/lib/log.sh
source "${REPO_ROOT_WC}/test/lima/lib/log.sh"
# shellcheck source=test/lima/lib/arch.sh
source "${REPO_ROOT_WC}/test/lima/lib/arch.sh"
# shellcheck source=test/lima/lib/remote.sh
source "${REPO_ROOT_WC}/test/lima/lib/remote.sh"
# shellcheck source=test/lima/lib/split-gate.sh
source "${REPO_ROOT_WC}/test/lima/lib/split-gate.sh"
# shellcheck source=test/lima/lib/install-split.sh
source "${REPO_ROOT_WC}/test/lima/lib/install-split.sh"

wc_repo_root() {
    lima_repo_root
}

wc_target_arch() {
    lima_target_arch
}

wc_lima_status() {
    lima_status "$1"
}

wc_remote() {
    lima_remote "$@"
}

wc_edgelet_bin() {
    lima_edgelet_bin "$@"
}

# List running workload container IDs in embedded k8s.io namespace (ctr, not crictl).
wc_cri_container_ids() {
    local _vm="$1"
    wc_remote "${_vm}" \
        "ctr --address /run/edgelet/containerd.sock -n k8s.io containers list -q 2>/dev/null | sort | tr '\n' ' '"
}

# wc_wait_embedded_control_ready VM [TIMEOUT] — data + control units, CRI socket, API.
wc_wait_embedded_control_ready() {
    local _vm="$1" _timeout="${2:-120}" _elapsed=0
    while (( _elapsed < _timeout )); do
        if wc_remote "${_vm}" \
            "systemctl is-active --quiet edgelet-containerd \
             && systemctl is-active --quiet edgelet \
             && test -S /run/edgelet/containerd.sock"; then
            if wc_wait_edgelet_api "${_vm}" 30; then
                return 0
            fi
        fi
        sleep 2
        _elapsed=$(( _elapsed + 2 ))
    done
    return 1
}

# wc_wait_cri_ids_unchanged VM BEFORE_IDS [TIMEOUT] — stable ctr IDs after control restart.
wc_wait_cri_ids_unchanged() {
    local _vm="$1" _before="$2" _timeout="${3:-120}" _elapsed=0 _after=""
    while (( _elapsed < _timeout )); do
        _after="$(wc_cri_container_ids "${_vm}")"
        if [[ -n "${_after// /}" && "${_before}" == "${_after}" ]]; then
            printf '%s' "${_after}"
            return 0
        fi
        sleep 2
        _elapsed=$(( _elapsed + 2 ))
    done
    return 1
}

# wc_ms_local_running VM MS_NAME — MS listed in local source with running state.
wc_ms_local_running() {
    local _vm="$1" _ms="$2"
    wc_remote "${_vm}" \
        "edgelet ms ls --source local 2>/dev/null | grep -F '${_ms}' | grep -qi running"
}

wc_docker_ms_present() {
    local _vm="$1" _ms="${2:-workload-ms}"
    wc_ms_local_running "${_vm}" "${_ms}"
}

wc_docker_engine_active() {
    local _vm="$1"
    wc_remote "${_vm}" "edgelet system status 2>/dev/null | grep -q 'runtime.engine: docker'"
}

# wc_docker_ms_container_id VM MS_NAME — running local MS container ID (column 5).
wc_docker_ms_container_id() {
    local _vm="$1" _ms="$2"
    wc_remote "${_vm}" \
        "edgelet ms ls --source local 2>/dev/null | grep -F '${_ms}' | grep -i running | awk '{print \$5; exit}'"
}

# wc_wait_docker_ms_container_id VM MS_NAME [EXPECTED_CID] [TIMEOUT]
wc_wait_docker_ms_container_id() {
    local _vm="$1" _ms="$2" _expected="${3:-}" _timeout="${4:-120}"
    local _elapsed=0 _cid=""
    while (( _elapsed < _timeout )); do
        _cid="$(wc_docker_ms_container_id "${_vm}" "${_ms}")"
        if [[ -n "${_cid}" ]]; then
            printf '%s' "${_cid}"
            return 0
        fi
        if [[ -n "${_expected}" ]] \
            && wc_remote "${_vm}" "docker ps -q --filter id=${_expected} 2>/dev/null | grep -q ."; then
            printf '%s' "${_expected}"
            return 0
        fi
        sleep 2
        _elapsed=$(( _elapsed + 2 ))
    done
    return 1
}

wc_embedded_split_active() {
    lima_embedded_split_active "$1"
}

wc_embedded_ms_running() {
    local _vm="$1"
    local _ids
    _ids="$(wc_cri_container_ids "${_vm}")"
    [[ -n "${_ids// /}" ]]
}

wc_embedded_split_gate() {
    local _vm="$1"
    wc_embedded_split_active "${_vm}" && wc_embedded_ms_running "${_vm}"
}

wc_deploy_ms() {
    local _vm="$1" _fixture="$2" _ms="${3:-workload-ms}"
    local _ssh_config _ssh_host

    _ssh_config="$(lima_ssh_config "${_vm}")"
    _ssh_host="$(lima_ssh_host "${_vm}")"

    scp -F "${_ssh_config}" -q "${_fixture}" "${_ssh_host}:/tmp/workload-ms.yaml"
    wc_remote "${_vm}" "docker pull docker.io/library/alpine:3.19 >/dev/null 2>&1 || true"
    wc_remote "${_vm}" "edgelet deploy -f /tmp/workload-ms.yaml"

    local _i
    for _i in $(seq 1 60); do
        if wc_ms_local_running "${_vm}" "${_ms}"; then
            return 0
        fi
        sleep 3
    done
    return 1
}

wc_wait_edgelet_api() {
    lima_wait_edgelet_api "$@"
}

wc_install_embedded_split() {
    lima_install_embedded_split "$1" "$2" "$3" "dev-wc"
}

# ensure_docker_vm REPO_ROOT VM_DOCKER SKIP_SETUP FIXTURE
ensure_docker_vm() {
    local _root="$1" _vm="$2" _skip_setup="$3" _fixture="$4"
    local _ms="workload-ms"

    command -v limactl >/dev/null || die "limactl required for T11-A"

    if [[ "${_skip_setup}" != true ]]; then
        "${_root}/test/engine-lifecycle/setup.sh"
    fi
    "${_root}/test/engine-lifecycle/vm-start.sh" --vm-name="${_vm}"

    if wc_remote "${_vm}" "systemctl is-active --quiet edgelet" \
        && wc_docker_engine_active "${_vm}" \
        && wc_docker_ms_present "${_vm}" "${_ms}"; then
        log_info "Docker VM ${_vm} already has edgelet + ${_ms} — skipping reinstall"
        return 0
    fi

    if wc_remote "${_vm}" "systemctl is-active --quiet edgelet" \
        && wc_docker_engine_active "${_vm}"; then
        log_info "Docker VM ${_vm} active but ${_ms} missing — deploying only"
        wc_wait_edgelet_api "${_vm}" 120 || die "edgelet not ready on docker VM"
        wc_deploy_ms "${_vm}" "${_fixture}" "${_ms}" || die "failed to deploy ${_ms} on docker VM"
        for _i in $(seq 1 15); do
            if wc_docker_ms_present "${_vm}" "${_ms}"; then
                log_ok "Docker VM ${_vm} ready for T11-A"
                return 0
            fi
            sleep 2
        done
        die "no running ${_ms} after deploy"
    fi

    log_step "Preparing docker engine + workload MS on ${_vm}"
    "${_root}/test/engine-lifecycle/vm-install.sh" --vm-name="${_vm}" --start-engine=docker
    wc_wait_edgelet_api "${_vm}" 120 || die "edgelet not ready on docker VM"
    wc_deploy_ms "${_vm}" "${_fixture}" "${_ms}" || die "failed to deploy ${_ms} on docker VM"
    for _i in $(seq 1 15); do
        if wc_docker_ms_present "${_vm}" "${_ms}"; then
            log_ok "Docker VM ${_vm} ready for T11-A"
            return 0
        fi
        sleep 2
    done
    die "no running ${_ms} after deploy"
}

# ensure_embedded_split REPO_ROOT VM_EMBED ARCH FIXTURE [STRICT]
ensure_embedded_split() {
    local _root="$1" _vm="$2" _arch="$3" _fixture="$4" _strict="${5:-false}"

    command -v limactl >/dev/null || {
        [[ "${_strict}" == true ]] && die "limactl required for embedded tests"
        log_warn "limactl not found — cannot run T11-C/D"
        return 1
    }

    "${_root}/test/embedded/vm-start.sh" --vm-name="${_vm}"

    if wc_embedded_split_gate "${_vm}"; then
        log_info "Embedded split + MS already present on ${_vm}"
        return 0
    fi

    if wc_embedded_split_active "${_vm}" && ! wc_embedded_ms_running "${_vm}"; then
        log_info "Split active but no MS — deploying workload-ms"
        wc_deploy_ms "${_vm}" "${_fixture}" || {
            [[ "${_strict}" == true ]] && die "MS deploy failed on ${_vm}"
            return 1
        }
        return 0
    fi

    wc_install_embedded_split "${_root}" "${_vm}" "${_arch}"
    wc_deploy_ms "${_vm}" "${_fixture}" || {
        [[ "${_strict}" == true ]] && die "MS deploy failed after split install"
        return 1
    }

    wc_embedded_split_gate "${_vm}" || {
        [[ "${_strict}" == true ]] && die "embedded split gate failed after prep"
        return 1
    }
    log_ok "Embedded VM ${_vm} ready for T11-C/D"
    return 0
}
