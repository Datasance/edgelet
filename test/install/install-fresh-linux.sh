#!/usr/bin/env bash
# test/install/install-fresh-linux.sh — fresh binary install + install receipt (linux, root).
#
# Usage:
#   sudo ./test/install/install-fresh-linux.sh
#   sudo ./test/install/install-fresh-linux.sh --arch=arm64
#
# Requires: build/edgelet-linux-<arch> (make build-linux-amd64 or build-all-archs).

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
INSTALL_SH="${REPO_ROOT}/install.sh"
RECEIPT="/var/backups/edgelet/install-receipt"

ARCH=""
for arg in "$@"; do
    case "${arg}" in
        --arch=*) ARCH="${arg#*=}" ;;
        -h|--help)
            echo "Usage: sudo $0 [--arch=amd64|arm64|arm|riscv64]"
            exit 0
            ;;
    esac
done

if [[ -z "${ARCH}" ]]; then
    case "$(uname -m)" in
        x86_64|amd64) ARCH=amd64 ;;
        aarch64|arm64) ARCH=arm64 ;;
        armv7l|armv6l|arm) ARCH=arm ;;
        riscv64) ARCH=riscv64 ;;
        *) echo "ERROR: unsupported uname -m: $(uname -m)" >&2; exit 1 ;;
    esac
fi

if [[ "$(uname -s)" != "Linux" ]]; then
    echo "ERROR: linux only (use Lima VM on macOS)" >&2
    exit 1
fi

if [[ "$(id -u)" -ne 0 ]]; then
    echo "ERROR: run as root (sudo $0)" >&2
    exit 1
fi

BIN="${REPO_ROOT}/build/edgelet-linux-${ARCH}"
[[ -f "${BIN}" ]] || { echo "ERROR: missing ${BIN}; run: make build-linux-${ARCH}" >&2; exit 1; }
[[ -x "${INSTALL_SH}" ]] || { echo "ERROR: missing ${INSTALL_SH}" >&2; exit 1; }

echo ">>> Fresh install (arch=${ARCH}, version=dev-it-fresh)"
"${INSTALL_SH}" --bin-path="${BIN}" --version=dev-it-fresh --arch="${ARCH}"

[[ -f "${RECEIPT}" ]] || { echo "ERROR: missing ${RECEIPT}" >&2; exit 1; }
grep -q '^installed_version=dev-it-fresh$' "${RECEIPT}" || {
    echo "ERROR: receipt missing installed_version=dev-it-fresh" >&2
    cat "${RECEIPT}" >&2
    exit 1
}
grep -q '^install_method=install' "${RECEIPT}" || {
    echo "ERROR: receipt missing install_method=install" >&2
    exit 1
}

command -v edgelet >/dev/null || { echo "ERROR: edgelet not in PATH" >&2; exit 1; }
[[ -x /usr/local/bin/edgelet ]] || { echo "ERROR: /usr/local/bin/edgelet missing" >&2; exit 1; }

echo ">>> PASS: fresh install + receipt"
