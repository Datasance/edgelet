#!/usr/bin/env bash
# test/init/lib/stage-install-bundle.sh
# Build a minimal repo layout for install.sh inside a Lima VM (Plan 10 IT).
# Source from test/init/*-smoke.sh — do not execute directly.

# validate_install_bundle_sources REPO_ROOT
validate_install_bundle_sources() {
    local _root="$1"
    local _missing=""
    for _p in \
        "${_root}/install.sh" \
        "${_root}/uninstall.sh" \
        "${_root}/scripts/lib/init-detect.sh" \
        "${_root}/scripts/lib/init-edgelet.sh" \
        "${_root}/scripts/edgelet-shutdown" \
        "${_root}/packaging/init/systemd/edgelet.service" \
        "${_root}/packaging/init/systemd/edgelet-containerd.service"
    do
        [ -f "${_p}" ] || _missing="${_missing} ${_p}"
    done
    if [ -n "${_missing}" ]; then
        echo "ERROR: missing install bundle inputs:${_missing}" >&2
        return 1
    fi
}

# stage_install_bundle_ssh SSH_CONFIG SSH_HOST STAGE_DIR REPO_ROOT BIN_PATH
# Copies install.sh + helpers + packaging/init preserving SCRIPT_DIR layout.
stage_install_bundle_ssh() {
    local _ssh_config="$1" _ssh_host="$2" _stage="$3" _root="$4" _bin="$5"
    local _tmpdir
    _tmpdir="$(mktemp -d)"
    # shellcheck disable=SC2064
    trap "rm -rf '${_tmpdir}'" RETURN

    mkdir -p "${_tmpdir}/scripts/lib" "${_tmpdir}/packaging"
    cp "${_root}/install.sh" "${_root}/uninstall.sh" "${_tmpdir}/"
    cp "${_bin}" "${_tmpdir}/edgelet"
    cp "${_root}/scripts/lib/"*.sh "${_tmpdir}/scripts/lib/"
    install -m 755 "${_root}/scripts/edgelet-shutdown" "${_tmpdir}/scripts/edgelet-shutdown"
    cp -R "${_root}/packaging/init" "${_tmpdir}/packaging/"

    ssh -F "${_ssh_config}" "${_ssh_host}" "rm -rf '${_stage}' && mkdir -p '${_stage}'"
    # COPYFILE_DISABLE avoids macOS xattr noise in GNU tar on the VM.
    COPYFILE_DISABLE=1 tar -C "${_tmpdir}" -cf - . | ssh -F "${_ssh_config}" "${_ssh_host}" "tar -xf - -C '${_stage}'"
}

# wait_edgelet_api_ready CMD...
# Polls "edgelet system status" until success or timeout (default 120s).
wait_edgelet_api_ready() {
    local _cmd="$1" _timeout="${2:-120}" _elapsed=0
    while (( _elapsed < _timeout )); do
        if eval "${_cmd}" >/dev/null 2>&1; then
            return 0
        fi
        sleep 2
        _elapsed=$(( _elapsed + 2 ))
    done
    echo "ERROR: edgelet API not ready after ${_timeout}s" >&2
    eval "${_cmd}" 2>&1 || true
    return 1
}
