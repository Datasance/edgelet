#!/bin/sh
# assemble-install.sh — splice embedded blocks into install.sh.
#
# Authoring inputs (read-only):
#   scripts/lib/init-detect.sh, scripts/lib/init-edgelet.sh
#   packaging/init/**
#   scripts/edgelet-shutdown
#   uninstall.sh
#
# Output: install.sh (root) — replaces ASSEMBLE:* regions.

set -e

ROOT=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
INSTALL_SH="${ROOT}/install.sh"
GEN_EMBEDDED="${ROOT}/scripts/install/gen-embedded-block.sh"
GEN_UNINSTALL="${ROOT}/scripts/install/gen-embedded-uninstall-block.sh"
GEN_INSTALL_SELF="${ROOT}/scripts/install/gen-embedded-install-self-block.sh"

[ -f "$INSTALL_SH" ] || { echo "ERROR: missing ${INSTALL_SH}" >&2; exit 1; }

for _gen in "$GEN_EMBEDDED" "$GEN_UNINSTALL" "$GEN_INSTALL_SELF"; do
    [ -f "$_gen" ] || { echo "ERROR: missing ${_gen}" >&2; exit 1; }
    [ -x "$_gen" ] || chmod +x "$_gen"
done

splice_region() {
    _src="$1"
    _begin="$2"
    _end="$3"
    _embed="$4"
    _out="$5"

    awk -v beg="$_begin" -v end="$_end" -v ef="$_embed" '
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
    ' "$_src" >"$_out"
}

OUT=$(mktemp)
trap 'rm -f "$OUT"' EXIT

EMBED=$(mktemp)
"$GEN_EMBEDDED" >"$EMBED"
splice_region "$INSTALL_SH" '# ASSEMBLE:EMBEDDED_BEGIN' '# ASSEMBLE:EMBEDDED_END' "$EMBED" "$OUT"
mv "$OUT" "$INSTALL_SH"
rm -f "$EMBED"

EMBED=$(mktemp)
"$GEN_UNINSTALL" >"$EMBED"
splice_region "$INSTALL_SH" '# ASSEMBLE:UNINSTALL_BEGIN' '# ASSEMBLE:UNINSTALL_END' "$EMBED" "$OUT"
mv "$OUT" "$INSTALL_SH"
rm -f "$EMBED"

EMBED=$(mktemp)
"$GEN_INSTALL_SELF" >"$EMBED"
splice_region "$INSTALL_SH" '# ASSEMBLE:INSTALL_SELF_BEGIN' '# ASSEMBLE:INSTALL_SELF_END' "$EMBED" "$OUT"
mv "$OUT" "$INSTALL_SH"
rm -f "$EMBED"

chmod +x "$INSTALL_SH"
echo ">>> Assembled embedded blocks into ${INSTALL_SH}"
