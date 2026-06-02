#!/bin/bash
# Plan 10-8: verify fat edgelet and staged embed ELFs are statically linked.
set -e

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "${ROOT}"

ARCH="${ARCH:-$(go env GOARCH 2>/dev/null || echo amd64)}"
FAT="${1:-build/bin/edgelet}"
STAGE_DIR="${2:-build/stage/bin}"

# k3s-root aux ships shell helpers (iptables-apply, *.sh); only gate ELF binaries.
is_elf_binary() {
    _path="$1"
    [ -f "${_path}" ] || return 1
    if command -v file >/dev/null 2>&1; then
        case "$(file -b "${_path}" 2>/dev/null)" in
            ELF*) return 0 ;;
        esac
        return 1
    fi
    # file(1) missing: ELF magic
    [ "$(head -c 4 "${_path}" 2>/dev/null)" = "$(printf '\177ELF')" ]
}

check_elf_static() {
    _path="$1"
    _label="$2"
    if [ ! -f "${_path}" ]; then
        echo "ERROR: missing ${_label}: ${_path}" >&2
        return 1
    fi
    if command -v file >/dev/null 2>&1; then
        _desc="$(file -b "${_path}")"
        if echo "${_desc}" | grep -q 'dynamically linked'; then
            echo "ERROR: ${_label} is dynamically linked: ${_path}" >&2
            echo "  ${_desc}" >&2
            return 1
        fi
        if ! echo "${_desc}" | grep -q 'statically linked'; then
            echo "ERROR: ${_label} is not statically linked: ${_path}" >&2
            echo "  ${_desc}" >&2
            return 1
        fi
    else
        if readelf -l "${_path}" 2>/dev/null | grep -q 'INTERP'; then
            echo "ERROR: ${_label} has dynamic interpreter: ${_path}" >&2
            return 1
        fi
    fi
    echo "OK: ${_label} static — ${_path}"
}

check_elf_static "${FAT}" "fat edgelet"

if [ -d "${STAGE_DIR}" ]; then
    echo "=== embed bundle ELF scan (${STAGE_DIR}) ==="
    _failed=0
    while IFS= read -r -d '' _elf; do
        is_elf_binary "${_elf}" || continue
        if ! check_elf_static "${_elf}" "$(basename "${_elf}")"; then
            _failed=1
        fi
    done < <(find "${STAGE_DIR}" -type f ! -name '.sha256sums' ! -name '.links' -print0)
    [ "${_failed}" -eq 0 ] || exit 1
fi

echo "check-embed-static: OK (ARCH=${ARCH})"
