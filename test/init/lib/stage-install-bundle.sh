#!/usr/bin/env bash
# test/init/lib/stage-install-bundle.sh
# Build a minimal release-flat layout for install.sh inside a Lima VM (init IT).
# Source from test/init/*-smoke.sh — do not execute directly.

# validate_install_bundle_sources REPO_ROOT
validate_install_bundle_sources() {
    local _root="$1"
    local _missing=""
    for _p in \
        "${_root}/install.sh" \
        "${_root}/uninstall.sh"
    do
        [ -f "${_p}" ] || _missing="${_missing} ${_p}"
    done
    if [ -n "${_missing}" ]; then
        echo "ERROR: missing install bundle inputs:${_missing}" >&2
        return 1
    fi
}

# stage_optional_samples REPO_ROOT DEST_DIR
# Copies config/CA samples when present (dist release or packaging tree).
stage_optional_samples() {
    local _root="$1" _dest="$2"
    if [ -f "${_root}/dist/edgelet-config.yaml.sample" ]; then
        cp "${_root}/dist/edgelet-config.yaml.sample" "${_dest}/"
    elif [ -f "${_root}/packaging/edgelet/etc/edgelet/config.default.yaml" ]; then
        cp "${_root}/packaging/edgelet/etc/edgelet/config.default.yaml" \
            "${_dest}/edgelet-config.yaml.sample"
    fi
    if [ -f "${_root}/dist/edgelet-controller-ca.crt.sample" ]; then
        cp "${_root}/dist/edgelet-controller-ca.crt.sample" "${_dest}/"
    elif [ -f "${_root}/packaging/edgelet/etc/edgelet/controller-ca.sample.crt" ]; then
        cp "${_root}/packaging/edgelet/etc/edgelet/controller-ca.sample.crt" \
            "${_dest}/edgelet-controller-ca.crt.sample"
    fi
}

# stage_install_bundle_ssh SSH_CONFIG SSH_HOST STAGE_DIR REPO_ROOT BIN_PATH
# Copies install.sh + binary (+ optional samples) — no scripts/lib or packaging/init tree.
stage_install_bundle_ssh() {
    local _ssh_config="$1" _ssh_host="$2" _stage="$3" _root="$4" _bin="$5"
    local _tmpdir
    _tmpdir="$(mktemp -d)"
    # shellcheck disable=SC2064
    trap "rm -rf '${_tmpdir}'" RETURN

    mkdir -p "${_tmpdir}"
    cp "${_root}/install.sh" "${_root}/uninstall.sh" "${_tmpdir}/"
    cp "${_bin}" "${_tmpdir}/edgelet"
    stage_optional_samples "${_root}" "${_tmpdir}"

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
