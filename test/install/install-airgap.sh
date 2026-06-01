#!/usr/bin/env bash
# test/install/install-airgap.sh — airgap install with SHA256 verification (linux, root).
#
# Usage:
#   sudo ./test/install/install-airgap.sh
#   sudo ./test/install/install-airgap.sh --arch=arm64

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
            echo "Usage: sudo $0 [--arch=amd64|arm64]"
            exit 0
            ;;
    esac
done

if [[ -z "${ARCH}" ]]; then
    case "$(uname -m)" in
        x86_64|amd64) ARCH=amd64 ;;
        aarch64|arm64) ARCH=arm64 ;;
        *) echo "ERROR: unsupported arch; pass --arch=" >&2; exit 1 ;;
    esac
fi

if [[ "$(uname -s)" != "Linux" ]] || [[ "$(id -u)" -ne 0 ]]; then
    echo "ERROR: run on linux as root" >&2
    exit 1
fi

BIN="${REPO_ROOT}/build/edgelet-linux-${ARCH}"
[[ -f "${BIN}" ]] || { echo "ERROR: missing ${BIN}" >&2; exit 1; }

EXPECTED_SHA256=$(sha256sum "${BIN}" | awk '{print $1}')

echo ">>> Airgap install (arch=${ARCH}, sha256=${EXPECTED_SHA256:0:16}…)"
"${INSTALL_SH}" \
    --airgap \
    --bin-path="${BIN}" \
    --expected-sha256="${EXPECTED_SHA256}" \
    --version=dev-it-airgap \
    --arch="${ARCH}"

grep -q '^install_method=install-airgap$' "${RECEIPT}" || {
    echo "ERROR: receipt missing install_method=install-airgap" >&2
    cat "${RECEIPT}" >&2
    exit 1
}

echo ">>> PASS: airgap install"
