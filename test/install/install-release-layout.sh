#!/usr/bin/env bash
# test/install/install-release-layout.sh — release-flat install from install.sh + binary only.
#
# Proves the monolithic install.sh needs no sibling scripts/lib or packaging/init tree.
#
# Usage:
#   ./scripts/release-binaries.sh v1.0.0-beta.1-test   # when build/edgelet-linux-<arch> exists
#   sudo ./test/install/install-release-layout.sh
#   sudo ./test/install/install-release-layout.sh --arch=arm64
#
# Requires: Linux, root (uid 0).
# On macOS (Darwin): run inside a Lima VM with the repo home mount, e.g.:
#   make build-linux-arm64
#   ./scripts/release-binaries.sh v1.0.0-beta.1-test
#   limactl shell edgelet-test -- sudo bash -c \
#     'cd '"$(pwd)"' && ./test/install/install-release-layout.sh --arch=arm64'

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
RECEIPT="/var/backups/edgelet/install-receipt"
VERSION="dev-it-release-layout"

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
        *) echo "ERROR: unsupported uname -m: $(uname -m); pass --arch=" >&2; exit 1 ;;
    esac
fi

if [[ "$(uname -s)" != "Linux" ]]; then
    echo "ERROR: linux only — on macOS run inside Lima (see script header)" >&2
    exit 1
fi

if [[ "$(id -u)" -ne 0 ]]; then
    echo "ERROR: run as root (sudo $0)" >&2
    exit 1
fi

BIN="${REPO_ROOT}/build/edgelet-linux-${ARCH}"
[[ -f "${BIN}" ]] || {
    echo "ERROR: missing ${BIN}; run: make build-linux-${ARCH}" >&2
    exit 1
}

DIST_INSTALL="${REPO_ROOT}/dist/install.sh"
if [[ ! -f "${DIST_INSTALL}" ]]; then
    echo ">>> dist/install.sh missing — running scripts/release-binaries.sh v1.0.0-beta.1-test"
    "${REPO_ROOT}/scripts/release-binaries.sh" "v1.0.0-beta.1-test"
fi
[[ -f "${DIST_INSTALL}" ]] || { echo "ERROR: missing ${DIST_INSTALL}" >&2; exit 1; }

STAGE="$(mktemp -d /tmp/edgelet-release-layout.XXXXXX)"
cleanup() { rm -rf "${STAGE}"; }
trap cleanup EXIT

cp "${DIST_INSTALL}" "${STAGE}/install.sh"
chmod 755 "${STAGE}/install.sh"
cp "${BIN}" "${STAGE}/edgelet-linux-${ARCH}"
chmod 755 "${STAGE}/edgelet-linux-${ARCH}"

echo ">>> Release-layout install (stage=${STAGE}, arch=${ARCH}, version=${VERSION})"
echo ">>> Stage contents (install.sh + binary only):"
ls -la "${STAGE}"

"${STAGE}/install.sh" \
    --bin-path="${STAGE}/edgelet-linux-${ARCH}" \
    --version="${VERSION}" \
    --arch="${ARCH}"

[[ -f "${RECEIPT}" ]] || { echo "ERROR: missing ${RECEIPT}" >&2; exit 1; }
grep -q "^installed_version=${VERSION}\$" "${RECEIPT}" || {
    echo "ERROR: receipt missing installed_version=${VERSION}" >&2
    cat "${RECEIPT}" >&2
    exit 1
}
grep -q '^install_method=install$' "${RECEIPT}" || {
    echo "ERROR: receipt missing install_method=install" >&2
    exit 1
}

command -v edgelet >/dev/null || { echo "ERROR: edgelet not in PATH" >&2; exit 1; }
[[ -x /usr/local/bin/edgelet ]] || { echo "ERROR: /usr/local/bin/edgelet missing" >&2; exit 1; }

if command -v systemctl >/dev/null 2>&1 && [[ -d /etc/systemd/system ]]; then
    systemctl is-active --quiet edgelet || {
        echo "ERROR: edgelet.service not active" >&2
        systemctl status edgelet --no-pager 2>&1 || true
        exit 1
    }
    echo ">>> edgelet.service is active"
fi

[[ -d /usr/share/edgelet/lib ]] && {
    echo "ERROR: /usr/share/edgelet/lib must not exist after install" >&2
    exit 1
}
[[ -d /usr/share/edgelet/init ]] && {
    echo "ERROR: /usr/share/edgelet/init must not exist after install" >&2
    exit 1
}

[[ -f /usr/share/edgelet/install.sh ]] || {
    echo "ERROR: /usr/share/edgelet/install.sh missing (OTA path)" >&2
    exit 1
}

echo ">>> PASS: release-layout install (install.sh + binary only)"
