#!/usr/bin/env bash
# test/lima/lib/install-split.sh — production install.sh split path for Lima IT.
# Source only; requires test/embedded/lib/log.sh and test/init/lib/stage-install-bundle.sh.

# lima_edgelet_bin REPO_ROOT ARCH
lima_edgelet_bin() {
    echo "${1}/build/edgelet-linux-${2}"
}

# lima_install_embedded_split REPO_ROOT VM ARCH [VERSION]
# Stops existing units, stages install bundle, runs install.sh --container-engine=edgelet.
lima_install_embedded_split() {
    local _root="$1" _vm="$2" _arch="$3" _version="${4:-dev-lima}"
    local _bin _stage _ssh_config _ssh_host

    _bin="$(lima_edgelet_bin "${_root}" "${_arch}")"
    [[ -f "${_bin}" ]] || die "Missing ${_bin}; run test/embedded/build.sh first"

    # shellcheck source=test/init/lib/stage-install-bundle.sh
    source "${_root}/test/init/lib/stage-install-bundle.sh"
    validate_install_bundle_sources "${_root}"
    [[ -f "${_root}/packaging/init/systemd/edgelet-containerd.service" ]] \
        || die "Missing edgelet-containerd.service (runtime split)"

    _stage="/tmp/edgelet-lima-install"
    _ssh_config="$(lima_ssh_config "${_vm}")"
    _ssh_host="$(lima_ssh_host "${_vm}")"

    log_step "Installing edgelet via install.sh split on ${_vm}"
    lima_remote "${_vm}" "systemctl stop edgelet edgelet-containerd 2>/dev/null || true"
    stage_install_bundle_ssh "${_ssh_config}" "${_ssh_host}" "${_stage}" "${_root}" "${_bin}"

    lima_remote "${_vm}" "
        set -e
        chmod +x ${_stage}/install.sh ${_stage}/edgelet ${_stage}/scripts/edgelet-shutdown
        ${_stage}/install.sh --bin-path=${_stage}/edgelet --version=${_version} --arch=${_arch} --container-engine=edgelet
    "

    lima_wait_split_units "${_vm}" 180 \
        || die "edgelet-containerd.service or edgelet.service not active after install.sh"
    lima_wait_edgelet_api "${_vm}" 180 \
        || die "edgelet API/runtime not ready after split install"
    log_ok "Embedded split install complete on ${_vm}"
}
