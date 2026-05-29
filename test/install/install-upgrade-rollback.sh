#!/usr/bin/env bash
# test/install/install-upgrade-rollback.sh — upgrade then rollback via install.sh (linux, root).
#
# Usage:
#   sudo ./test/install/install-upgrade-rollback.sh
#   sudo ./test/install/install-upgrade-rollback.sh --arch=arm64
#
# Uses the same build binary with distinct version labels to exercise receipt/cache/rollback.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
INSTALL_SH="${REPO_ROOT}/install.sh"
RECEIPT="/var/backups/edgelet/install-receipt"
PREVIOUS="/var/backups/edgelet/previous-release"
CACHE_DIR="/var/backups/edgelet/cache"

ARCH=""
for arg in "$@"; do
    case "${arg}" in
        --arch=*) ARCH="${arg#*=}" ;;
        -h|--help)
            echo "Usage: sudo $0 [--arch=amd64|arm64]"
            exit 0
            ;;
    esac
done

if [[ -z "${ARCH}" ]]; then
    case "$(uname -m)" in
        x86_64|amd64) ARCH=amd64 ;;
        aarch64|arm64) ARCH=arm64 ;;
        *) echo "ERROR: set --arch= explicitly for this arch" >&2; exit 1 ;;
    esac
fi

if [[ "$(uname -s)" != "Linux" ]]; then
    echo "ERROR: linux only" >&2
    exit 1
fi
if [[ "$(id -u)" -ne 0 ]]; then
    echo "ERROR: run as root" >&2
    exit 1
fi

BIN="${REPO_ROOT}/build/edgelet-linux-${ARCH}"
[[ -f "${BIN}" ]] || { echo "ERROR: missing ${BIN}" >&2; exit 1; }

V_BASE="dev-it-ota"
V_A="${V_BASE}-a"
V_B="${V_BASE}-b"

echo ">>> Install baseline ${V_A}"
"${INSTALL_SH}" --bin-path="${BIN}" --version="${V_A}" --arch="${ARCH}"

echo ">>> Upgrade to ${V_B}"
"${INSTALL_SH}" --upgrade --bin-path="${BIN}" --version="${V_B}" --arch="${ARCH}"

grep -q "^installed_version=${V_B}\$" "${RECEIPT}" || {
    echo "ERROR: receipt not at ${V_B}" >&2
    cat "${RECEIPT}" >&2
    exit 1
}
[[ -f "${PREVIOUS}" ]] || { echo "ERROR: missing ${PREVIOUS}" >&2; exit 1; }
grep -q "^previous_version=${V_A}\$" "${PREVIOUS}" || {
    echo "ERROR: previous-release missing ${V_A}" >&2
    cat "${PREVIOUS}" >&2
    exit 1
}
_cached="${CACHE_DIR}/edgelet-${V_A}-linux-${ARCH}"
[[ -f "${_cached}" ]] || { echo "ERROR: missing cached binary ${_cached}" >&2; exit 1; }

echo ">>> Rollback to ${V_A}"
"${INSTALL_SH}" --rollback

grep -q "^installed_version=${V_A}\$" "${RECEIPT}" || {
    echo "ERROR: receipt not rolled back to ${V_A}" >&2
    cat "${RECEIPT}" >&2
    exit 1
}

echo ">>> PASS: upgrade + rollback"
