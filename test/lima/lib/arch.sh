#!/usr/bin/env bash
# test/lima/lib/arch.sh — target Linux arch for Lima IT. Source only.

# lima_target_arch — linux/arm64 or linux/amd64 from host CPU.
lima_target_arch() {
    local _host
    _host="$(lima_native_arch)"
    case "${_host}" in
        arm64|aarch64) echo "arm64" ;;
        x86_64) echo "amd64" ;;
        *) echo "amd64" ;;
    esac
}

# lima_native_arch — real CPU arch (Rosetta-safe on Apple Silicon).
lima_native_arch() {
    local _arch
    _arch="$(uname -m)"
    if [[ "${_arch}" == "x86_64" && "$(uname -s)" == "Darwin" ]]; then
        if sysctl -n hw.optional.arm64 2>/dev/null | grep -q 1; then
            echo "arm64"
            return
        fi
    fi
    echo "${_arch}"
}
