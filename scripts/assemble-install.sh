#!/bin/sh
# assemble-install.sh — splice self-contained init/shutdown block into install.sh.
#
# Authoring inputs (read-only):
#   scripts/lib/init-detect.sh, scripts/lib/init-edgelet.sh
#   packaging/init/**
#   scripts/edgelet-shutdown
#
# Output: install.sh (root) — replaces # ASSEMBLE:EMBEDDED_BEGIN … END region.

set -e

ROOT=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
INSTALL_SH="${ROOT}/install.sh"
GEN="${ROOT}/scripts/install/gen-embedded-block.sh"
BEGIN_MARKER='# ASSEMBLE:EMBEDDED_BEGIN'
END_MARKER='# ASSEMBLE:EMBEDDED_END'

[ -f "$INSTALL_SH" ] || { echo "ERROR: missing ${INSTALL_SH}" >&2; exit 1; }
[ -f "$GEN" ] || { echo "ERROR: missing ${GEN}" >&2; exit 1; }
[ -x "$GEN" ] || chmod +x "$GEN"

grep -qF "$BEGIN_MARKER" "$INSTALL_SH" || {
    echo "ERROR: ${INSTALL_SH} missing ${BEGIN_MARKER}" >&2
    exit 1
}
grep -qF "$END_MARKER" "$INSTALL_SH" || {
    echo "ERROR: ${INSTALL_SH} missing ${END_MARKER}" >&2
    exit 1
}

EMBED=$(mktemp)
trap 'rm -f "$EMBED"' EXIT

"$GEN" >"$EMBED"

OUT=$(mktemp)
trap 'rm -f "$EMBED" "$OUT"' EXIT

awk -v beg="$BEGIN_MARKER" -v end="$END_MARKER" -v ef="$EMBED" '
    BEGIN { skip = 0 }
    $0 == beg {
        print
        while ((getline line < ef) > 0) {
            print line
        }
        close(ef)
        skip = 1
        next
    }
    skip && $0 == end {
        skip = 0
        print
        next
    }
    skip { next }
    { print }
' "$INSTALL_SH" >"$OUT"

mv "$OUT" "$INSTALL_SH"
chmod +x "$INSTALL_SH"
echo ">>> Assembled embedded init block into ${INSTALL_SH}"
